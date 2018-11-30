package epay

import (
	"testing"
)

func TestNew(t *testing.T) {
	api, err := New("cin", "test")
	if err != nil {
		t.Fatalf("expected to pass, but got %v", err)
	}

	if api.url != ePayURL {
		t.Fatalf("expected URL to be %q, but got %q", ePayURL, api.url)
	}

	if expected := "test"; api.secret != expected {
		t.Fatalf("expected secret to be %q, but got %q", expected, api.secret)
	}
}

func TestWithDemoURL(t *testing.T) {
	api, err := New("cin", "test", WithDemoURL())
	if err != nil {
		t.Fatalf("expected to pass, but got %v", err)
	}

	if api.url != ePayDemoURL {
		t.Fatalf("expected URL to be %q, but got %q", ePayDemoURL, api.url)
	}
}
