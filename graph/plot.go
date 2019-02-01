package graph

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/pkg/errors"
	"go.cryptoscope.co/ssb"
	"gonum.org/v1/gonum/graph"
	"gonum.org/v1/gonum/graph/encoding"
	"gonum.org/v1/gonum/graph/encoding/dot"
	"gonum.org/v1/gonum/graph/simple"
)

func (g *Graph) NodeCount() int {
	return len(g.lookup)
}

func (g *Graph) RenderSVG(w io.Writer) error {
	dotbytes, err := dot.Marshal(g, "trust", "", "")
	if err != nil {
		return errors.Wrap(err, "dot marshal failed")
	}
	dotR := bytes.NewReader(dotbytes)

	dotCmd := exec.Command("dot", "-Tsvg")
	dotCmd.Stdout = w
	dotCmd.Stdin = dotR
	// dotCmd.Stdin = io.TeeReader(dotR, dotFile)
	return errors.Wrap(dotCmd.Run(), "RenderSVG: dot command failed")
}

func (g *Graph) RenderSVGToFile(fname string) error {
	os.Remove(fname)
	svgFile, err := os.Create(fname)
	if err != nil {
		return errors.Wrap(err, "svg file create failed")
	}
	defer svgFile.Close()
	return g.RenderSVG(svgFile)
}

// https://www.graphviz.org/doc/info/attrs.html
var (
	_ encoding.Attributer = (*contactNode)(nil)
	_ encoding.Attributer = (*contactEdge)(nil)
)

func (g *Graph) Attributes() []encoding.Attribute {
	return []encoding.Attribute{
		{Key: "rankdir", Value: "LR"},
	}
}

type contactNode struct {
	graph.Node
	feed *ssb.FeedRef
}

func (n contactNode) String() string {
	// TODO: inject about/name service
	return n.feed.Ref()[:8]
}

func (n contactNode) Attributes() []encoding.Attribute {
	return []encoding.Attribute{
		{Key: "label", Value: fmt.Sprintf("%q", n.String())},
	}
}

type contactEdge struct {
	simple.WeightedEdge
	isBlock bool
}

func (n contactEdge) Attributes() []encoding.Attribute {
	c := "black"
	if n.W > 1 {
		c = "firebrick1"
	}
	return []encoding.Attribute{
		{Key: "color", Value: c},
		// {Key: "label", Value: fmt.Sprintf(`"%f"`, n.W)},
	}
}
