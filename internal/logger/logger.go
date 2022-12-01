package logger

import "fmt"

const (
	ErrorLevel = iota
	WarningLevel
	InfoLevel
	DebugLevel
)

var Level int

func Debug(args ...any) {
	if Level > 2 {
		fmt.Println(append([]any{"[DEBUG]"}, args...)...)
	}
}

func Info(args ...any) {
	if Level > 1 {
		fmt.Println(append([]any{"[INFO]"}, args...)...)
	}
}

func Warn(args ...any) {
	if Level > 0 {
		fmt.Println(append([]any{"[WARN]"}, args...)...)
	}
}

func Error(args ...any) {
	fmt.Println(append([]any{"[ERROR]"}, args...)...)
}
