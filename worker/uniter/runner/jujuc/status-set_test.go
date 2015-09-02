// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"launchpad.net/goyaml"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

type statusSetSuite struct {
	ContextSuite
}

var _ = gc.Suite(&statusSetSuite{})

func (s *statusSetSuite) SetUpTest(c *gc.C) {
	s.ContextSuite.SetUpTest(c)
}

var statusSetInitTests = []struct {
	args []string
	err  string
}{
	{[]string{"maintenance"}, ""},
	{[]string{"maintenance", ""}, ""},
	{[]string{"maintenance", "hello"}, ""},
	{[]string{}, `invalid args, require <status> \[message\] \[data\]`},
	{[]string{"maintenance", "hello", "", "extra"}, `unrecognized args: \["extra"\]`},
	{[]string{"foo", "hello"}, `invalid status "foo", expected one of \[maintenance blocked waiting active\]`},
}

func (s *statusSetSuite) TestStatusSetInit(c *gc.C) {
	for i, t := range statusSetInitTests {
		c.Logf("test %d: %#v", i, t.args)
		hctx := s.GetStatusHookContext(c)
		com, err := jujuc.NewCommand(hctx, cmdString("status-set"))
		c.Assert(err, jc.ErrorIsNil)
		testing.TestInit(c, com, t.args, t.err)
	}
}

func (s *statusSetSuite) TestHelp(c *gc.C) {
	hctx := s.GetStatusHookContext(c)
	com, err := jujuc.NewCommand(hctx, cmdString("status-set"))
	c.Assert(err, jc.ErrorIsNil)
	ctx := testing.Context(c)
	code := cmd.Main(com, ctx, []string{"--help"})
	c.Assert(code, gc.Equals, 0)
	expectedHelp := "" +
		"usage: status-set [options] <maintenance | blocked | waiting | active> [message] [data]\n" +
		"purpose: set status information\n" +
		"\n" +
		"options:\n" +
		"--service  (= false)\n" +
		"    set this status for the service to which the unit belongs if the unit is the leader\n" +
		"\n" +
		"Sets the workload status of the charm. Message is optional.\n" +
		"The \"last updated\" attribute of the status is set, even if the\n" +
		"status and message are the same as what's already set.\n"

	c.Assert(bufferString(ctx.Stdout), gc.Equals, expectedHelp)
	c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
}

func (s *statusSetSuite) TestStatus(c *gc.C) {
	for i, args := range [][]string{
		[]string{"maintenance", "doing some work"},
		[]string{"active", ""},
		[]string{"maintenance", "valid data", `Information:
  number: 22
  string: Something
OtherInformation:
  number: 24
  string: SomethingElse`},
	} {
		c.Logf("test %d: %#v", i, args)
		hctx := s.GetStatusHookContext(c)
		com, err := jujuc.NewCommand(hctx, cmdString("status-set"))
		c.Assert(err, jc.ErrorIsNil)
		ctx := testing.Context(c)
		code := cmd.Main(com, ctx, args)
		c.Assert(code, gc.Equals, 0)
		c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
		c.Assert(bufferString(ctx.Stdout), gc.Equals, "")
		status, err := hctx.UnitStatus()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(status.Status, gc.Equals, args[0])
		c.Assert(status.Info, gc.Equals, args[1])
		if len(args) == 3 {
			text, err := goyaml.Marshal(status.Data)
			c.Check(err, jc.ErrorIsNil)
			c.Assert(string(text), gc.Equals, args[2]+"\n")
		}
	}
}

func (s *statusSetSuite) TestServiceStatus(c *gc.C) {
	for i, args := range [][]string{
		[]string{"--service", "maintenance", "doing some work"},
		[]string{"--service", "active", ""},
		[]string{"--service", "maintenance", "valid data", `Information:
  number: 22
  string: Something
OtherInformation:
  number: 24
  string: SomethingElse`},
	} {
		c.Logf("test %d: %#v", i, args)
		hctx := s.GetStatusHookContext(c)
		com, err := jujuc.NewCommand(hctx, cmdString("status-set"))
		c.Assert(err, jc.ErrorIsNil)
		ctx := testing.Context(c)
		code := cmd.Main(com, ctx, args)
		c.Assert(code, gc.Equals, 0)
		c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
		c.Assert(bufferString(ctx.Stdout), gc.Equals, "")
		status, err := hctx.ServiceStatus()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(status.Service.Status, gc.Equals, args[1])
		c.Assert(status.Service.Info, gc.Equals, args[2])
		if len(args) == 4 {
			text, err := goyaml.Marshal(status.Service.Data)
			c.Check(err, jc.ErrorIsNil)
			c.Assert(string(text), gc.Equals, args[3]+"\n")
		}
		c.Assert(status.Units, jc.DeepEquals, []jujuc.StatusInfo{})

	}
}

func (s *statusSetSuite) TestStatusInvalidData(c *gc.C) {
	for i, args := range [][]string{
		[]string{"maintenance", "valid data", `InvalidInformation:
  22: number
  string: Something
OtherInformation:
  23: 24
  string: SomethingElse`},
	} {
		c.Logf("test %d: %#v", i, args)
		hctx := s.GetStatusHookContext(c)
		com, err := jujuc.NewCommand(hctx, cmdString("status-set"))
		c.Assert(err, jc.ErrorIsNil)
		ctx := testing.Context(c)
		code := cmd.Main(com, ctx, args)
		c.Assert(code, gc.Equals, 2)
		c.Assert(bufferString(ctx.Stderr), gc.Equals, "error: cannot parse data to set status: cannot process data: keys must be strings\n")
		c.Assert(bufferString(ctx.Stdout), gc.Equals, "")
	}
}
