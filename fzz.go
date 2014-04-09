package main

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"syscall"
	"unsafe"
)

const (
	ansiEraseDisplay = "\033[2J"
	ansiResetCursor  = "\033[H"
	carriageReturn   = "\015"
	defaultPrompt    = ">> "
)

var originalSttyState bytes.Buffer
var winRows uint16
var winCols uint16

type winsize struct {
	rows, cols, xpixel, ypixel uint16
}

func getWinsize() winsize {
	ws := winsize{}
	syscall.Syscall(syscall.SYS_IOCTL,
		uintptr(0), uintptr(syscall.TIOCGWINSZ),
		uintptr(unsafe.Pointer(&ws)))
	return ws
}

// TODO: This is wrong: stdin should be the TTY
func getSttyState(state *bytes.Buffer) (err error) {
	cmd := exec.Command("stty", "-g")
	cmd.Stdin = os.Stdin
	cmd.Stdout = state
	return cmd.Run()
}

// TODO: This is wrong: stdin and stdout should be the TTY
func setSttyState(state *bytes.Buffer) (err error) {
	cmd := exec.Command("stty", state.String())
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	return cmd.Run()
}

func NewTTY() (t *TTY, err error) {
	fh, err := os.OpenFile("/dev/tty", os.O_RDWR, 0666)
	if err != nil {
		return
	}
	t = &TTY{fh, defaultPrompt}
	return
}

type TTY struct {
	*os.File
	prompt string
}

// Clears the screen and sets the cursor to first row, first column
func (t *TTY) resetScreen() {
	fmt.Fprint(t.File, ansiEraseDisplay+ansiResetCursor)
}

// Print prompt with `in`
func (t *TTY) printPrompt(in []byte) {
	fmt.Fprintf(t.File, t.prompt+"%s", in)
}

// Positions the cursor after the prompt and `inlen` colums to the right
func (t *TTY) cursorAfterPrompt(inlen int) {
	t.setCursorPos(0, len(t.prompt)+inlen)
}

// Sets the cursor to `line` and `col`
func (t *TTY) setCursorPos(line int, col int) {
	fmt.Fprintf(t.File, "\033[%d;%dH", line+1, col+1)
}

func init() {
	ws := getWinsize()
	winRows = ws.rows
	winCols = ws.cols
}

func main() {
	err := getSttyState(&originalSttyState)
	if err != nil {
		log.Fatal(err)
	}
	// TODO: this needs to be run when the process is interrupted
	defer setSttyState(&originalSttyState)

	setSttyState(bytes.NewBufferString("cbreak"))
	setSttyState(bytes.NewBufferString("-echo"))

	cmdTemplate := "ag {{}}"
	placeholder := "{{}}"

	tty, err := NewTTY()
	if err != nil {
		log.Fatal(err)
	}

	printer := NewPrinter(tty, int(winCols), int(winRows)-3)

	runner := &Runner{
		printer:     printer,
		template:    cmdTemplate,
		placeholder: placeholder,
		buf:         new(bytes.Buffer),
	}

	// TODO: Clean this up. This is a mess.
	var input []byte = make([]byte, 0)
	var b []byte = make([]byte, 1)

	for {
		tty.resetScreen()
		tty.printPrompt(input[:len(input)])

		if len(input) > 0 {
			runner.killCurrent()

			fmt.Fprintf(tty, "\n")

			go func() {
				runner.runWithInput(input[:len(input)])
				tty.cursorAfterPrompt(len(input))
			}()
		}

		os.Stdin.Read(b)
		switch b[0] {
		case 127:
			// Backspace
			if len(input) > 1 {
				input = input[:len(input)-1]
			} else if len(input) == 1 {
				input = nil
			}
		case 4, 10, 13:
			// Ctrl-D, line feed, carriage return
			runner.writeCmdStdout(os.Stdout)
			return
		default:
			// TODO: Default is wrong here. Only append printable characters to
			// input
			input = append(input, b...)
		}
	}
}
