package logger

import "fmt"

var Level int

func Debug(args ...any) {
	if Level > 2 {
		fmt.Println(append([]any{"[DEBUG]"}, args...)...)
	}
}

func Info(args ...any) {
	fmt.Println(append([]any{"[INFO]"}, args...)...)
}

func Error(args ...any) {
	fmt.Println(append([]any{"[ERROR]"}, args...)...)
}

