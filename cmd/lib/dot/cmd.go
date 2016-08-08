package dot

import (
	"fmt"
	"github.com/codegangsta/cli"
	"gopkg.in/sensorbee/sensorbee.v0/bql/parser"
	"io/ioutil"
	"os"
	"path/filepath"
)

func SetUp() cli.Command {
	cmd := cli.Command{
		Name:        "dot",
		Usage:       "make DOT file",
		Description: "dot command make a DOT file representing BQL graph",
		Action:      Run,
	}
	cmd.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "topology, t",
			Value: "",
			Usage: "name of the topology, the BQL file name is used as topology name on default)",
		},
	}
	return cmd
}

func Run(c *cli.Context) error {
	if len(c.Args()) != 1 {
		cli.ShowSubcommandHelp(c)
		os.Exit(1)
	}
	if err := makeDotFile(c); err != nil {
		return cli.NewExitError(err.Error(), 1)
	}
	return nil
}

func makeDotFile(c *cli.Context) error {
	bqlFile := c.Args()[0]
	topologyName := filepath.Base(bqlFile)
	topologyName = topologyName[:len(topologyName)-len(filepath.Ext(topologyName))]
	if n := c.String("topology"); n != "" {
		topologyName = n
	}

	queries, err := func() (string, error) {
		f, err := os.Open(bqlFile)
		if err != nil {
			return "", err
		}
		defer f.Close()
		b, err := ioutil.ReadAll(f)
		if err != nil {
			return "", err
		}
		return string(b), nil
	}()
	if err != nil {
		return err
	}
	bp := parser.New()
	stmts, err := bp.ParseStmts(queries)
	if err != nil {
		return err
	}
	dot := ""
	for _, stmt := range stmts {
		dot += makeDotLine(stmt)
	}
	dot = fmt.Sprintf(`digraph bql_graph {
  graph [label = "%s", labelloc=t];
%s}
`, topologyName, dot)
	return ioutil.WriteFile(fmt.Sprintf("%s.dot", topologyName), []byte(dot), 0644)
}

func makeDotLine(stmt interface{}) string {
	dot := ""
	switch stmt := stmt.(type) {
	case parser.CreateSourceStmt:
		dot += fmt.Sprintf("  %s [shape = box];\n", stmt.Name)
		// add stmt.Type is better?
	case parser.CreateStreamAsSelectStmt:
		dot += fmt.Sprintf("  %s [shape = ellipse];\n", stmt.Name)
		for _, rel := range stmt.Select.Relations {
			// TODO: duplicate input
			switch rel.Type {
			case parser.ActualStream:
				dot += fmt.Sprintf("  %s -> %s;\n", rel.Name, stmt.Name)
			case parser.UDSFStream:
				fmt.Println("UDSF, function name: " + rel.Name)
			}
		}
	case parser.CreateSinkStmt:
		dot += fmt.Sprintf("  %s [shape = box];\n", stmt.Name)
	case parser.InsertIntoFromStmt:
		dot += fmt.Sprintf("  %s -> %s;\n", stmt.Input, stmt.Sink)
	case parser.CreateStateStmt:
		fmt.Println("CREATE STATE, name: " + stmt.Name)
	case parser.LoadStateStmt:
		fmt.Println("LOAD STATE, name: " + stmt.Name)
	case parser.LoadStateOrCreateStmt:
		fmt.Println("LOAD STATE OR CREATE, name: " + stmt.Name)
	}
	return dot
}
