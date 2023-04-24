package exampleLibrary

import (
	log "github.com/sirupsen/logrus"
)

func Add1000(in int) int {
	log.Printf("Adding 1000 to %v", in)
	return in + 1000
}
