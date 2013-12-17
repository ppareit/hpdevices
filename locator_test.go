package hpdevices

// +build ignore

import (
	"fmt"
	"testing"
)

func _Test_Locator(t *testing.T) {
	d, err := LocalizeDevice()
	if err == nil {
		fmt.Println("HPDevice found at", d.URL)
	} else {
		fmt.Println("HPDevice error", err)
	}
}
