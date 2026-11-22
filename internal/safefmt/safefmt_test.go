package safefmt

import (
	"fmt"
	"strings"
	"testing"
)

func TestSprintMatchesFmtForCommonValues(t *testing.T) {
	type point struct{ X, Y int }
	values := []any{
		nil,
		42,
		-1,
		uint8(7),
		3.14,
		float32(1.5),
		"hello",
		"",
		true,
		[]int{1, 2, 3},
		[]string{},
		[]any{1, "two", false},
		[2]int{4, 5},
		map[string]any{"a": 1, "b": "x"},
		map[string]any{},
		map[string]int{"z": 26, "a": 1},
		point{1, 2},
		[]any{map[string]any{"k": []int{7}}},
		fmt.Errorf("boom"),
	}
	for _, value := range values {
		want := fmt.Sprintf("%v", value)
		if got := Sprint(value); got != want {
			t.Errorf("Sprint(%#v) = %q, want fmt's %q", value, got, want)
		}
	}
}

type stringyType struct{ n int }

func (s stringyType) String() string { return fmt.Sprintf("stringer<%d>", s.n) }

func TestSprintUsesStringer(t *testing.T) {
	if got, want := Sprint(stringyType{3}), "stringer<3>"; got != want {
		t.Fatalf("Sprint(Stringer) = %q, want %q", got, want)
	}
}

type panicStringer struct{}

func (panicStringer) String() string { panic("boom") }

func TestSprintSurvivesPanickingStringer(t *testing.T) {
	got := Sprint(panicStringer{})
	if !strings.Contains(got, "PANIC") {
		t.Fatalf("Sprint(panicking Stringer) = %q, want a PANIC marker", got)
	}
}

func TestSprintCyclicMap(t *testing.T) {
	m := map[string]any{"name": "root"}
	m["self"] = m
	got := Sprint(m)
	// The fatal failure this guards against is a stack overflow; any output
	// containing a cycle marker and returning at all is the contract
	if !strings.Contains(got, "<cycle>") {
		t.Fatalf("Sprint(cyclic map) = %q, want a <cycle> marker", got)
	}
}

func TestSprintCyclicSlice(t *testing.T) {
	s := make([]any, 1)
	s[0] = s
	got := Sprint(s)
	if !strings.Contains(got, "<cycle>") {
		t.Fatalf("Sprint(cyclic slice) = %q, want a <cycle> marker", got)
	}
}

type node struct {
	Next *node
	Name string
}

func TestSprintCyclicPointerGraph(t *testing.T) {
	n := &node{Name: "a"}
	n.Next = &node{Name: "b", Next: n}
	got := Sprint(n)
	if !strings.Contains(got, "a") || !strings.Contains(got, "b") {
		t.Fatalf("Sprint(cyclic pointers) = %q, want both node names", got)
	}
	if !strings.Contains(got, "<cycle>") {
		t.Fatalf("Sprint(cyclic pointers) = %q, want a <cycle> marker", got)
	}
}

func TestSprintCyclicValueBehindInterfaces(t *testing.T) {
	m := map[string]any{}
	m["self"] = []any{m}
	got := Sprint(m)
	if !strings.Contains(got, "<cycle>") {
		t.Fatalf("Sprint(cycle through interface) = %q, want a <cycle> marker", got)
	}
}

func TestSprintSharedSubvalueIsNotACycle(t *testing.T) {
	shared := map[string]int{"k": 1}
	holder := map[string]any{"a": shared, "b": shared}
	got := Sprint(holder)
	if strings.Contains(got, "<cycle>") {
		t.Fatalf("Sprint(DAG) = %q, shared subvalue must not be marked a cycle", got)
	}
	if strings.Count(got, "k:1") != 2 {
		t.Fatalf("Sprint(DAG) = %q, want the shared map rendered twice", got)
	}
}

func TestSprintEmptyContainersOfSameType(t *testing.T) {
	got := Sprint([]any{[]int{}, []int{}})
	if got != "[[] []]" {
		t.Fatalf("Sprint(empty slices) = %q, want %q", got, "[[] []]")
	}
}

func TestSprintDeepNestingTruncates(t *testing.T) {
	var value any = 1
	for i := 0; i < 500; i++ {
		value = []any{value}
	}
	got := Sprint(value)
	if !strings.Contains(got, "...") {
		t.Fatalf("Sprint(500-deep) = %.80q..., want a truncation marker", got)
	}
	if len(got) > 1000 {
		t.Fatalf("Sprint(500-deep) produced %d bytes, want a bounded dump", len(got))
	}
}

func TestSprintStructWithUnexportedFields(t *testing.T) {
	type hidden struct {
		Exported   string
		unexported string
	}
	got := Sprint(hidden{Exported: "x", unexported: "y"})
	if !strings.Contains(got, "x") || !strings.Contains(got, "?") {
		t.Fatalf("Sprint(unexported fields) = %q, want exported value and a placeholder", got)
	}
}

func TestSprintDeterministicMapOrder(t *testing.T) {
	m := map[string]int{"b": 2, "a": 1, "c": 3}
	first := Sprint(m)
	for i := 0; i < 20; i++ {
		if got := Sprint(m); got != first {
			t.Fatalf("Sprint(map) is not deterministic: %q vs %q", got, first)
		}
	}
}
