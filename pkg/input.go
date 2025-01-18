package pkg

import (
	"bufio"
	"os"
	"strings"
)

type InputHandler struct {
	reader *bufio.Reader
}

func NewInputHandler() *InputHandler {
	return &InputHandler{
		reader: bufio.NewReader(os.Stdin),
	}
}

func (ih *InputHandler) ReadInput() (string, error) {
	input, err := ih.reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(input), nil
}

func (ih *InputHandler) ProcessInput(input string, dm *DisplayManager) bool {
	return dm.HandleInput(input)
}
