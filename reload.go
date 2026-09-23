package cfggo

import "reflect"

// Reload reloads the configuration from all sources
func (c *Structure) Reload() error {
	c.ensureInit()

	// Hold reloadMutex for the snapshot/reset, loader, and stage phases so a
	// concurrent Set cannot land mid-reload (where it would be silently lost
	// to a rollback or overwritten by the override restore). The lock is
	// released before validators run so a validator may safely call Set or
	// Reload, and re-acquired only for the atomic commit. It is released
	// before OnChange callbacks run so a callback may safely call Set
	oldConfig, cand, err := func() (map[string]interface{}, reloadCandidate, error) {
		// Deferred so a panic in user conversion code (UnmarshalText on a
		// custom type) cannot leave the reload lock held forever
		c.reloadMutex.Lock()
		defer c.reloadMutex.Unlock()
		return c.reloadLocked()
	}()
	if err != nil {
		return err
	}

	// Validate the private candidate with no cfggo lock held: validators are
	// arbitrary user code that may call Set or Reload. Live state is already
	// the last-known-good snapshot at this point, so a failed validation is
	// a plain return — nothing was published, no rollback is needed. The
	// validators see a copy, so a container a validator retains never
	// aliases live configuration once the candidate is committed
	if err := c.validateSnapshot(cloneInterfaceMap(cand.data), cand.prov); err != nil {
		c.log().Warn("cfggo: configuration validation failed after reload; keeping previous values", "err", err)
		return err
	}

	// Commit the validated candidate atomically, re-applying any Set deltas
	// that landed during the validation window. A false return reports that this
	// reload was superseded by a newer commit (optimistic-concurrency loss) and
	// must neither commit nor notify: live state is already fresher
	committed := func() bool {
		c.reloadMutex.Lock()
		defer c.reloadMutex.Unlock()
		return c.commitReloadLocked(cand)
	}()
	if !committed {
		return nil
	}

	// Notify OnChange listeners with the aggregate set of keys whose values
	// differ from the pre-reload snapshot
	// Skip the (allocating) diff entirely
	// when nobody is listening.
	if c.hasListeners() {
		c.notifyChange(c.changeSet(oldConfig))
	}
	return nil
}

// reloadCandidate holds the staged reloaded state while validators run
// unlocked. It is private to the reload and never aliased with live state
type reloadCandidate struct {
	data       map[string]interface{}
	prov       map[string]Source
	trail      map[string][]Source
	loaded     map[string]interface{}
	changed    bool
	ver        uint64
	preChanged bool
	preVer     uint64
	// gen is the reload generation captured at stage (under reloadMutex). When
	// the committer finds c.reloadGen != gen, another reload committed during
	// this reload's validation window and this candidate is stale
	gen uint64
	// preSet snapshots the pre-reload values of SourceSet keys so commit can
	// tell a validation-window Set (live value differs) from an untouched
	// pre-reload override (live value identical)
	preSet map[string]interface{}
}

// reloadLocked performs the reload phases. The caller must hold reloadMutex.
// It returns the pre-reload snapshot (for change notification), the staged
// candidate, and any loader error (with live left untouched).
//
// The source document is read first, with no cfggo lock held, so a slow file
// or HTTP endpoint never blocks accessor reads. The candidate is then built
// under configMutex in one critical section: live is swapped out for fresh
// maps, the file, environment and runtime-override layers are applied to
// those, and live is swapped back in before the lock is released. No reader
// or Set can therefore observe an intermediate state (such as the defaults
// the candidate starts from), which an earlier design exposed while the
// layers were applied to live one lock acquisition at a time
func (c *Structure) reloadLocked() (map[string]interface{}, reloadCandidate, error) {
	// Phase 1: fetch the source document without holding any lock
	var data []byte
	if c.configHandler != nil {
		d, err := c.readConfigSource()
		if err != nil {
			c.log().Error("cfggo: failed to reload configuration source", "err", err)
			return nil, reloadCandidate{}, err
		}
		data = d
	}

	// Phase 2: build the candidate under the write lock. The pre-reload maps
	// are set aside (never mutated) so a failure simply puts them back
	c.configMutex.Lock()
	defer c.configMutex.Unlock()

	oldConfig := c.configData
	oldProvenance := c.provenance
	oldTrail := c.provenanceTrail
	oldLoaded := c.loadedData
	oldChanged := c.changed
	oldChangeVersion := c.changeVersion

	// The dirty flag is left alone: a reload neither creates nor discards
	// unsaved Set values (they are re-asserted below), so whatever was
	// pending stays pending
	restoreLive := func() {
		c.configData = oldConfig
		c.provenance = oldProvenance
		c.provenanceTrail = oldTrail
		c.loadedData = oldLoaded
		c.changed = oldChanged
		c.changeVersion = oldChangeVersion
	}

	// Start the candidate from the defaults. A Structure that was never
	// initialised has no defaults; start from a private copy of whatever it
	// holds instead so the pre-reload maps stay untouched either way
	if c.defaultData != nil {
		c.resetToDefaultsLocked()
	} else {
		c.configData = cloneInterfaceMap(oldConfig)
		if c.configData == nil {
			c.configData = make(map[string]interface{})
		}
		c.provenance = cloneSourceMap(oldProvenance)
		c.provenanceTrail = cloneSourceTrailMap(oldTrail)
		c.loadedData = cloneInterfaceMap(oldLoaded)
	}

	if c.configHandler != nil {
		if err := c.loadJSONConfigFromBytes(data, true); err != nil {
			c.log().Error("cfggo: failed to reload configuration source", "err", err)
			restoreLive()
			return nil, reloadCandidate{}, err
		}
	}

	if err := c.loadFromEnvLocked(); err != nil {
		c.log().Error("cfggo: failed to reload environment variables", "err", err)
		restoreLive()
		return nil, reloadCandidate{}, err
	}

	// Re-assert runtime override precedence. The standard flag package will
	// not re-run an already-parsed flag set, so the file and environment
	// layers reloaded above would otherwise clobber values supplied on the
	// command line. Programmatic Set values are also runtime overrides and
	// must survive reloads until the caller changes them again
	for key, src := range oldProvenance {
		if src != SourceFlag && src != SourceSet {
			continue
		}
		if v, ok := oldConfig[key]; ok {
			if err := c.set(key, v); err != nil {
				c.log().Warn("cfggo: could not restore runtime override during reload", "key", key, "source", src, "err", c.redactSecretValueError(key, err))
				continue
			}
			c.recordSourceLocked(key, src)
		}
	}

	if err := c.checkUnrecognizedKeysLocked(); err != nil {
		c.log().Warn("cfggo: unrecognized configuration keys after reload; keeping previous values", "err", err)
		restoreLive()
		return nil, reloadCandidate{}, err
	}

	// The accessor closures installed during Init read c.configData live on
	// every call, so the reloaded values become visible at commit without
	// reinstalling them. Re-running replaceConfigFuncs here would rewrite the
	// struct func fields, racing with any goroutine currently calling an
	// accessor (and with a concurrent reload)

	// Stage: hand the candidate maps out and put the pre-reload maps back as
	// live. From here on live is last-known-good; validation runs unlocked
	// against the candidate maps, which nothing else references. The notify
	// baseline keeps its own clone because a validation-window Set mutates
	// live in place after the lock is released
	var cand reloadCandidate
	cand.gen = c.reloadGen
	cand.data = c.configData
	cand.prov = c.provenance
	cand.trail = c.provenanceTrail
	cand.loaded = c.loadedData
	cand.changed = c.changed
	cand.ver = c.changeVersion
	cand.preChanged = oldChanged
	cand.preVer = oldChangeVersion
	for key, src := range oldProvenance {
		if src == SourceSet {
			if v, ok := oldConfig[key]; ok {
				if cand.preSet == nil {
					cand.preSet = make(map[string]interface{})
				}
				cand.preSet[key] = cloneMutableInterface(v)
			}
		}
	}
	notify := cloneInterfaceMap(oldConfig)
	restoreLive()

	return notify, cand, nil
}

// commitReloadLocked publishes the validated candidate atomically and
// re-applies any SourceSet writes that landed during the validation window
// (a Set then targets last-known-good and would otherwise be overwritten by
// the commit). The caller must hold reloadMutex. It reports whether the
// candidate was committed: false means another reload committed during this
// reload's unlocked validation window (c.reloadGen advanced past cand.gen),
// so this candidate was staged from a stale baseline and is discarded —
// committing it would clobber the fresher live state (lost update). The
// superseded caller returns success without notifying: live is already newer
// than what it staged, and changeSet(oldConfig) would misreport the diff
func (c *Structure) commitReloadLocked(cand reloadCandidate) bool {
	if c.reloadGen != cand.gen {
		return false
	}
	c.reloadGen++
	c.configMutex.Lock()
	defer c.configMutex.Unlock()
	var delta map[string]interface{}
	// added collects keys that did not exist when the candidate was staged
	// (a NewFlag registered during the validation window): the candidate
	// cannot know them, so they are carried over with their provenance
	type addedKey struct {
		value interface{}
		src   Source
	}
	var added map[string]addedKey
	for key, v := range c.configData {
		if _, staged := cand.data[key]; !staged {
			if added == nil {
				added = make(map[string]addedKey)
			}
			added[key] = addedKey{value: cloneMutableInterface(v), src: c.provenance[key]}
			continue
		}
		if c.provenance[key] != SourceSet {
			continue
		}
		if pre, ok := cand.preSet[key]; ok && reflect.DeepEqual(pre, v) {
			continue
		}
		if delta == nil {
			delta = make(map[string]interface{})
		}
		delta[key] = cloneMutableInterface(v)
	}
	windowTouched := c.changeVersion != cand.preVer
	liveChanged := c.changed
	// The version only ever moves forward: a Save that captured the live
	// version during the validation window must not later match a rewound
	// value and clear the dirty flag over a Set it never wrote
	liveVersion := c.changeVersion
	c.configData = cand.data
	c.provenance = cand.prov
	c.provenanceTrail = cand.trail
	c.loadedData = cand.loaded
	c.changed = cand.changed || liveChanged
	if cand.ver > liveVersion {
		c.changeVersion = cand.ver
	} else {
		c.changeVersion = liveVersion
	}
	if windowTouched {
		c.changed = true
	}
	for key, a := range added {
		c.configData[key] = a.value
		c.recordSourceLocked(key, a.src)
	}
	for key, v := range delta {
		if err := c.set(key, v); err != nil {
			c.log().Warn("cfggo: failed to set value", "key", key, "err", c.redactSecretValueError(key, err))
			continue
		}
		c.markChangedLocked()
		c.recordSourceLocked(key, SourceSet)
	}
	return true
}

func (c *Structure) resetToDefaultsLocked() {
	if c.defaultData == nil {
		return
	}
	c.configData = make(map[string]interface{}, len(c.defaultData))
	c.provenance = make(map[string]Source, len(c.defaultData))
	c.provenanceTrail = nil
	c.loadedData = nil
	for k, v := range c.defaultData {
		// Clone mutable values (maps, slices, pointers) so that a subsequent
		// mutation via an accessor or Set does not corrupt c.defaultData.
		c.configData[k] = cloneMutableInterface(v)
		c.provenance[k] = SourceDefault
	}
}

// changeSet compares the current configData against a previous snapshot and
// returns one Change per key whose value changed or was added, annotated with
// the current source
func (c *Structure) changeSet(old map[string]interface{}) []Change {
	c.configMutex.RLock()
	defer c.configMutex.RUnlock()

	var changes []Change
	for k, newVal := range c.configData {
		if oldVal, ok := old[k]; !ok || !reflect.DeepEqual(oldVal, newVal) {
			changes = append(changes, Change{
				Key:    k,
				Old:    cloneMutableInterface(old[k]),
				New:    cloneMutableInterface(newVal),
				Source: c.provenance[k],
			})
		}
	}
	for k, oldVal := range old {
		if _, ok := c.configData[k]; !ok {
			changes = append(changes, Change{
				Key:    k,
				Old:    cloneMutableInterface(oldVal),
				New:    nil,
				Source: SourceUnknown,
			})
		}
	}
	return changes
}
