package sio

import (
	"fmt"
	"os"
)

type Debugger interface {
	Log(main string, v ...any)
	withContext(context string) Debugger
}

type noopDebugger struct{}

func (d noopDebugger) Log(main string, _v ...any) {}

func (d noopDebugger) withContext(context string) Debugger { return d }

type printDebugger struct {
	context string
}

func NewPrintDebugger() Debugger {
	return new(printDebugger)
}

func (d *printDebugger) Log(main string, _v ...any) {
	fmt.Print(main)
	if len(d.context) != 0 {
		fmt.Print(d.context)
		fmt.Print(": ")
	}
	for _, v := range _v {
		fmt.Print(": ")
		fmt.Print(v)
	}
	fmt.Print("\n")
	os.Stdout.Sync()
}

func (d printDebugger) withContext(context string) Debugger {
	d.context = context
	return &d
}
