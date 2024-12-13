package log

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

const maxFileSize = 128 * 1024 // 128 KiB

var (
	instance *logger
	// ANSI escape codes
	Reset, Bold, Red, Green, Yellow, Blue string
)

// logger wraps an io.Writer, and implements locking and rotation
type logger struct {
	writer io.Writer
	mutex  sync.Mutex
	debug  bool
	// whether to output "interactive" messages like infos, warnings and errors
	interactive bool
}

func Init(w io.Writer, interactive bool, colors bool) {
	debug := os.Getenv("DEBUG") != ""
	instance = &logger{writer: w, debug: debug, interactive: interactive}
	if colors {
		Reset = "\033[0m"
		Bold = "\033[1m"
		Red = "\033[31m"
		Green = "\033[32m"
		Yellow = "\033[33m"
		Blue = "\033[36m"
	}
}

// Write implements io.Writer, locking and rotating as needed
func (l *logger) Write(bytes []byte) (int, error) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	l.tryRotate()
	return l.writer.Write(bytes)
}

func (l *logger) tryRotate() {
	f, ok := l.writer.(*os.File)
	if !ok {
		// Not a file, can't rotate
		return
	}
	info, err := f.Stat()
	if err != nil {
		return
	}
	if info.Size() < maxFileSize {
		// Not ripe for rotation
		return
	}
	if f.Truncate(0) == nil {
		f.Seek(0, 0)
	}
}

func timestamp() string {
	currentTime := time.Now()
	format := "15:04:05"
	if instance.debug {
		format = "15:04:05.000"
	}
	return "[" + currentTime.Format(format) + "]"
}

func Debugf(format string, a ...any) {
	if !instance.debug || !instance.interactive {
		return
	}
	message := fmt.Sprintf(format, a...)
	fmt.Fprintf(instance, "%s DEBUG %s\n", timestamp(), message)
}

func Infof(format string, a ...any) {
	if !instance.interactive {
		return
	}
	message := fmt.Sprintf(format, a...)
	fmt.Fprintf(instance, "%s %sINFO%s %s\n", timestamp(), Bold+Blue, Reset, message)
}

func Warningf(format string, a ...any) {
	if !instance.interactive {
		return
	}
	message := fmt.Sprintf(format, a...)
	fmt.Fprintf(instance, "%s %sWARNING%s %s\n", timestamp(), Bold+Yellow, Reset, message)
}

func Errorf(format string, a ...any) {
	if !instance.interactive {
		return
	}
	message := fmt.Sprintf(format, a...)
	fmt.Fprintf(instance, "%s %sERROR%s %s\n", timestamp(), Bold+Red, Reset, message)
}

func Fatalf(format string, a ...any) {
	if instance.interactive {
		message := fmt.Sprintf(format, a...)
		fmt.Fprintf(instance, "%s %sFATAL%s %s\n", timestamp(), Bold+Red, Reset, message)
	}
	os.Exit(1)
}

// Printf writes a message without any formatting
func Printf(format string, a ...any) {
	if instance.interactive {
		fmt.Fprintf(instance, format, a...)
	}
}

// Emitf emits data, i.e. it should not be suppressed in non-interactive mode
func Emitf(format string, a ...any) {
	fmt.Fprintf(instance, format, a...)
}
