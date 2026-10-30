// Copyright 2026 Yigit Yargili
// SPDX-License-Identifier: Apache-2.0

package ask

import (
	"fmt"
	"runtime"

	llama "github.com/goccy/go-llama"
)

// Engine runs the fine-tuned GGUF model in process with a pure-Go llama.cpp.
type Engine struct {
	l *llama.Llama
	m *llama.Model
	c *llama.Context
}

// LoadEngine loads the model at path.
func LoadEngine(path string) (*Engine, error) {
	l, err := llama.New()
	if err != nil {
		return nil, fmt.Errorf("starting the model runtime: %w", err)
	}
	m, err := l.LoadModel(path)
	if err != nil {
		l.Close()
		return nil, fmt.Errorf("loading ask model %s: %w", path, err)
	}
	threads := min(8, max(1, runtime.NumCPU()/2))
	c, err := m.NewContext(llama.ContextParams{NCtx: 512, NThreads: uint32(threads)})
	if err != nil {
		m.Close()
		l.Close()
		return nil, fmt.Errorf("creating model context: %w", err)
	}
	return &Engine{l: l, m: m, c: c}, nil
}

// Complete greedily continues prompt up to the end of the line, constrained
// by the GBNF grammar.
func (e *Engine) Complete(prompt, grammar string) (string, error) {
	if err := e.c.Reset(); err != nil {
		return "", err
	}
	r, err := e.c.Generate(prompt, llama.Params{NPredict: 64, Stop: []string{"\n"}, Grammar: grammar})
	return r.Text, err
}

// Close frees the model.
func (e *Engine) Close() error {
	e.c.Close()
	e.m.Close()
	return e.l.Close()
}
