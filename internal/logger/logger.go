package logger

import "fmt"

const (
	ErrorLevel = Level(iota)
	WarningLevel
	InfoLevel
	DebugLevel
)

type Level int

var level = InfoLevel

func SetLevel(l Level) {
	level = l
}

func Debug(args ...any) {
	if level > 2 {
		fmt.Println(append([]any{"[DEBUG]"}, args...)...)
	}
}

func Info(args ...any) {
	if level > 1 {
		fmt.Println(append([]any{"[INFO]"}, args...)...)
	}
}

func Warn(args ...any) {
	if level > 0 {
		fmt.Println(append([]any{"[WARN]"}, args...)...)
	}
}

func Error(args ...any) {
	fmt.Println(append([]any{"[ERROR]"}, args...)...)
}
