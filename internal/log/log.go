package log

import (
	"fmt"
	"io"
	"os"
	"syscall"
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

const maxFileSize = 128 * 1024 // 128 KiB

var logFile io.Writer = os.Stdout
var debug = len(os.Getenv("DEBUG")) > 0

type logWriter struct {
	writer io.Writer
}

func (w logWriter) Write(bytes []byte) (int, error) {
	w.tryRotate()
	return w.writer.Write(bytes)
}

func (w logWriter) tryRotate() bool {
	f, ok := w.writer.(*os.File)
	if !ok {
		// Not a file, can't rotate
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	if info.Size() < maxFileSize {
		// Not ripe for rotation
		return false
	}
	syscall.Ftruncate(int(f.Fd()), 0)
	f.Seek(0, 0)
	return true
}

func timestamp() string {
	currentTime := time.Now()
	timestamp := "[" + currentTime.Format("15:04:05") + "]"
	return timestamp
}

func Debugf(format string, a ...any) {
	if !debug {
		return
	}
	message := fmt.Sprintf(format, a...)
	fmt.Fprintf(logFile, "%s DEBUG %s\n", timestamp(), message)
}

func Infof(format string, a ...any) {
	message := fmt.Sprintf(format, a...)
	fmt.Fprintf(logFile, "%s %sINFO%s %s\n", timestamp(), ColorBlue, ColorReset, message)
}

func Warningf(format string, a ...any) {
	message := fmt.Sprintf(format, a...)
	fmt.Fprintf(logFile, "%s %sWARNING%s %s\n", timestamp(), ColorYellow, ColorReset, message)
}

func Errorf(format string, a ...any) {
	message := fmt.Sprintf(format, a...)
	fmt.Fprintf(logFile, "%s %sERROR%s %s\n", timestamp(), ColorRed, ColorReset, message)
}

func Fatalf(format string, a ...any) {
	message := fmt.Sprintf(format, a...)
	fmt.Fprintf(logFile, "%s %sFATAL%s %s\n", timestamp(), ColorRed, ColorReset, message)
	os.Exit(1)
}

func SetOutput(writer io.Writer) {
	logFile = logWriter{writer: writer}
}
