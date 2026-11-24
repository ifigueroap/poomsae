package main

import (
	"github.com/ifigueroap/poomsae/internal/api"
	"github.com/ifigueroap/poomsae/internal/textsafe"
)

func federationEmbeddingLines(status *api.FederationEmbeddingStatus) []string {
	fields := status.DisplayFields()
	for i := range fields {
		fields[i] = textsafe.Line(fields[i])
	}
	return fields
}
