package multiplex

import "context"

const NO_VALUE = ""
const NO_PID = -1

type status int

type Multiplexer struct {
	ctx         context.Context
	rootPath    string
	envFiles    []string
	services    []*service
	executables []*executable
}

func NewMultiplexer(rootPath string, envFiles []string) *Multiplexer {
	return &Multiplexer{
		ctx:         context.Background(),
		rootPath:    rootPath,
		envFiles:    envFiles,
		services:    make([]*service, 0),
		executables: make([]*executable, 0),
	}
}
