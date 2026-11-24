package main

import (
	"github.com/ifigueroap/poomsae/internal/client"
	"github.com/ifigueroap/poomsae/internal/storageinspect"
)

// The poomsae binary links the storage engines, so it can check storage before
// it starts or replaces a local daemon. Installing at package initialization
// covers every command and the tests that drive them.
func init() {
	client.UseStorageInspector(storageinspect.Inspector{})
}
