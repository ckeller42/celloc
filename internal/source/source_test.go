package source_test

import (
	"testing"

	"github.com/ckeller42/celloc/internal/source"
)

func TestFix_HasFix(t *testing.T) {
	if (source.Fix{Mode: 2}).HasFix() != true {
		t.Fatal("mode 2 should be a fix")
	}
	if (source.Fix{Mode: 0}).HasFix() != false {
		t.Fatal("mode 0 must not be a fix")
	}
	if (source.Fix{Mode: 1}).HasFix() != false {
		t.Fatal("mode 1 (no fix) must not be a fix")
	}
}
