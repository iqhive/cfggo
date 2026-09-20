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
	c.reloadMutex.Lock()
	oldConfig, cand, err := c.reloadLocked()
	c.reloadMutex.Unlock()
	if err != nil {
		return err
	}

	// Validate the private candidate with no cfggo lock held: validators are
	// arbitrary user code that may call Set or Reload. Live state is already
	// the last-known-good snapshot at this point, so a failed validation is
	// a plain return — nothing was published, no rollback is needed
	if err := c.validateSnapshot(cand.data, cand.prov); err != nil {
		c.log().Warn("cfggo: configuration validation failed after reload; keeping previous values", "err", err)
		return err
	}

	// Commit the validated candidate atomically, re-applying any Set deltas
	// that landed during the validation window. A false return reports that this
	// reload was superseded by a newer commit (optimistic-concurrency loss) and
	// must neither commit nor notify: live state is already fresher
	c.reloadMutex.Lock()
	committed := c.commitReloadLocked(cand)
	c.reloadMutex.Unlock()
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
// candidate, and any loader error (with live already rolled back)
func (c *Structure) reloadLocked() (map[string]interface{}, reloadCandidate, error) {
	// First, make a copy of the current configuration for potential rollback
	var oldConfig map[string]interface{}
	// oldProvenance lets us re-assert command-line flag precedence after the
	// file/env layers below have been reloaded (see the restore step)
	var oldProvenance map[string]Source
	var oldTrail map[string][]Source
	var oldLoaded map[string]interface{}
	var oldChanged bool
	var oldChangeVersion uint64

	// Get a snapshot of the current configuration. The dirty flag is left
	// alone: a reload neither creates nor discards unsaved Set values (they
	// are re-asserted below), so whatever was pending stays pending
	c.configMutex.Lock()
	oldChanged = c.changed
	oldChangeVersion = c.changeVersion
	oldConfig = cloneInterfaceMap(c.configData)
	oldProvenance = cloneSourceMap(c.provenance)
	oldTrail = cloneSourceTrailMap(c.provenanceTrail)
	oldLoaded = cloneInterfaceMap(c.loadedData)
	c.resetToDefaultsLocked()
	c.configMutex.Unlock()

	rollback := func() {
		c.configMutex.Lock()
		c.configData = oldConfig
		c.provenance = oldProvenance
		c.provenanceTrail = oldTrail
		c.loadedData = oldLoaded
		c.changed = oldChanged
		c.changeVersion = oldChangeVersion
		c.configMutex.Unlock()
	}

	var err error

	// Attempt to reload configuration from file (loadConfig handles its own locking)
	if c.configHandler != nil {
		if err = c.loadConfig(false); err != nil {
			c.log().Error("cfggo: failed to reload configuration source", "err", err)

			// Rollback to old configuration on error. Restore provenance too so
			// it stays consistent with the values after a failed reload
			rollback()
			return nil, reloadCandidate{}, err
		}
	}

	// Reload from environment variables
	if err = c.loadFromEnv(); err != nil {
		c.log().Error("cfggo: failed to reload environment variables", "err", err)
		rollback()
		return nil, reloadCandidate{}, err
	}

	// Check if flags have been parsed before calling parseFlags
	var flagsParsed bool
	c.configMutex.RLock()
	flagsParsed = c.flagSet != nil && c.flagSet.Parsed()
	c.configMutex.RUnlock()

	// Reload from flags if they've been parsed
	if flagsParsed {
		if err := c.parseFlags(); err != nil {
			c.log().Warn("cfggo: could not re-parse command-line flags during reload", "err", err)
		}
	}

	// Re-assert runtime override precedence. The standard flag package will not
	// re-run an already-parsed flag set (parseFlags above early-returns), so the
	// file and environment layers reloaded above can otherwise clobber values
	// supplied on the command line. Programmatic Set values are also runtime
	// overrides and must survive reloads until the caller changes them again
	for key, src := range oldProvenance {
		if src != SourceFlag && src != SourceSet {
			continue
		}
		if v, ok := oldConfig[key]; ok {
			if err := c.applyLoaded(key, v, src); err != nil {
				c.log().Warn("cfggo: could not restore runtime override during reload", "key", key, "source", src, "err", err)
			}
		}
	}

	if err = c.checkUnrecognizedKeys(); err != nil {
		c.log().Warn("cfggo: unrecognized configuration keys after reload; keeping previous values", "err", err)
		rollback()
		return nil, reloadCandidate{}, err
	}

	// The accessor closures installed during Init read c.configData live on
	// every call, so the reloaded values are already visible without
	// reinstalling them. Re-running replaceConfigFuncs here would rewrite the
	// struct func fields, racing with any goroutine currently calling an
	// accessor (and with a concurrent reload)

	// Stage: extract the reloaded candidate into private maps and restore the
	// pre-reload snapshot as live, in a single configMutex critical section.
	// From here on live is last-known-good; validation runs unlocked against
	// the candidate map. The snapshot maps are returned to live by ownership
	// transfer (no second clone): from the snapshot point to here nothing
	// outside this critical section can observe or mutate them — loaders run
	// under reloadMutex (held), readers/Sets take configMutex (held), and no
	// reader handle was published — so they are still uniquely owned. The
	// notify baseline keeps its own clone because a validation-window Set
	// mutates live in place after this section releases the lock.
	var cand reloadCandidate
	c.configMutex.Lock()
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
	c.configData = oldConfig
	c.provenance = oldProvenance
	c.provenanceTrail = oldTrail
	c.loadedData = oldLoaded
	c.changed = oldChanged
	c.changeVersion = oldChangeVersion
	c.configMutex.Unlock()

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
	for key, v := range c.configData {
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
	c.configData = cand.data
	c.provenance = cand.prov
	c.provenanceTrail = cand.trail
	c.loadedData = cand.loaded
	c.changed = cand.changed || liveChanged
	c.changeVersion = cand.ver
	if windowTouched {
		c.changed = true
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
