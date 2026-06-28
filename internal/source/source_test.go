package source_test

import (
	"context"
	"testing"

	"github.com/ckeller42/celloc/internal/source"
)

type fakeSource struct {
	name string
	fix  source.Fix
	err  error
}

func (f fakeSource) Name() string                            { return f.name }
func (f fakeSource) Fix(context.Context) (source.Fix, error) { return f.fix, f.err }

func TestSelect_PrefersFirstWithFix(t *testing.T) {
	gnss := fakeSource{name: "gnss", err: source.ErrNoFix}
	cell := fakeSource{name: "cell", fix: source.Fix{Mode: 2, Lat: 1, Lon: 2}}
	got, err := source.Select(context.Background(), gnss, cell)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.Lat != 1 || got.Lon != 2 {
		t.Fatalf("got %+v, want cell fix", got)
	}
}

func TestSelect_PriorityOrderWins(t *testing.T) {
	// Proves the interface is genuinely pluggable: a higher-priority source with
	// a fix outranks a lower one. (Future GNSS over cell.)
	gnss := fakeSource{name: "gnss", fix: source.Fix{Mode: 2, Lat: 10}}
	cell := fakeSource{name: "cell", fix: source.Fix{Mode: 2, Lat: 20}}
	got, _ := source.Select(context.Background(), gnss, cell)
	if got.Lat != 10 {
		t.Fatalf("got lat %v, want gnss (10)", got.Lat)
	}
}

func TestSelect_NoneHaveFix(t *testing.T) {
	a := fakeSource{name: "a", err: source.ErrNoFix}
	b := fakeSource{name: "b", fix: source.Fix{Mode: 0}} // present but no fix
	if _, err := source.Select(context.Background(), a, b, nil); err != source.ErrNoFix {
		t.Fatalf("got %v, want ErrNoFix", err)
	}
}

func TestFix_HasFix(t *testing.T) {
	if (source.Fix{Mode: 2}).HasFix() != true {
		t.Fatal("mode 2 should be a fix")
	}
	if (source.Fix{Mode: 0}).HasFix() != false {
		t.Fatal("mode 0 should not be a fix")
	}
}
