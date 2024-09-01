package log

import (
	"fmt"
	"os"
	"time"
)

const (
	// ANSI escape codes for text colors
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorBlue   = "\033[36m"
)

func timestamp() string {
	currentTime := time.Now()
	timestamp := "[" + currentTime.Format("2006/01/02 15:04:05") + "]"
	return timestamp
}

func Infof(format string, a ...any) {
	message := fmt.Sprintf(format, a...)
	fmt.Printf("%sINFO%s %s %s\n", ColorBlue, ColorReset, timestamp(), message)
}

func Fatalf(format string, a ...any) {
	message := fmt.Sprintf(format, a...)
	fmt.Printf("%sFATAL%s %s %s\n", ColorRed, ColorReset, timestamp(), message)
	os.Exit(1)
}
