package main

import "testing"

func TestLangKeysMatch(t *testing.T) {
	for k := range msgs["en"] {
		if _, ok := msgs["de"][k]; !ok {
			t.Errorf("de missing %s", k)
		}
	}
	for k := range msgs["de"] {
		if _, ok := msgs["en"][k]; !ok {
			t.Errorf("en missing %s", k)
		}
	}
}

func TestAssetsExist(t *testing.T) {
	for c := range carriers {
		if svg("logos", c) == "" {
			t.Errorf("logo %s", c)
		}
	}
	for _, i := range statusIcon {
		if svg("icons", i) == "" {
			t.Errorf("icon %s", i)
		}
	}
	for _, i := range []string{"package", "plus", "refresh-cw", "trash-2", "map-pin", "languages"} {
		if svg("icons", i) == "" {
			t.Errorf("icon %s", i)
		}
	}
}
