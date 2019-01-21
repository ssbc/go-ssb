package graph

import (
	"bytes"
	"fmt"
	"os/exec"

	"github.com/pkg/errors"
	"gonum.org/v1/gonum/graph/encoding/dot"
)

func (g *Graph) Nodes() int {
	return len(g.lookup)
}

func (g *Graph) RenderSVG() error {
	dotbytes, err := dot.Marshal(g.dg, "trust", "", "")
	if err != nil {
		return errors.Wrap(err, "dot marshal failed")
	}
	dotCmd := exec.Command("dot", "-Tsvg", "-o", "fullgraph.svg")
	dotCmd.Stdin = bytes.NewReader(dotbytes)
	out, err := dotCmd.CombinedOutput()
	if err != nil {
		fmt.Println("dot out:", out)
		return errors.Wrap(err, "dot run failed")
	}
	return nil
}

var (
// _ dot.Attributers = (*Graph)(nil)
// _ dot.Structurer = (*Graph)(nil)
// _ dot.Porter = (*Graph)(nil)
)

// func (g *Graph) FromPort() (a, b string) {}
