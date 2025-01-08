package utils

import (
	"log"
	"os"
)

// Logger is a global logger instance.
var Logger *log.Logger

func init() {
	Logger = log.New(os.Stdout, "[snapshot-insight] ", log.LstdFlags|log.Lshortfile)
}
