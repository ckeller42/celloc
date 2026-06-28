package uciconf

import (
	"context"
	"errors"
	"testing"
)

func TestLoadWithSuccess(t *testing.T) {
	run := func(context.Context) (string, error) { return "geolocd.main.key='pk.z'", nil }
	cfg, err := loadWith(context.Background(), run)
	if err != nil || cfg.Key != "pk.z" {
		t.Fatalf("cfg=%+v err=%v", cfg, err)
	}
}

func TestLoadWithErrorReturnsDefaults(t *testing.T) {
	run := func(context.Context) (string, error) { return "", errors.New("uci missing") }
	cfg, err := loadWith(context.Background(), run)
	if err == nil {
		t.Fatal("want error propagated")
	}
	if cfg != Defaults() {
		t.Fatalf("want defaults on error, got %+v", cfg)
	}
}
