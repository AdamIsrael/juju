package main

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju/go/cmd"
	"launchpad.net/juju/go/juju"
)

// UnitAgent is a cmd.Command responsible for running a unit agent.
type UnitAgent struct {
	agent
	UnitName string
}

func NewUnitAgent() *UnitAgent {
	return &UnitAgent{agent: agent{name: "unit"}}
}

// Init initializes the command for running.
func (a *UnitAgent) Init(f *gnuflag.FlagSet, args []string) error {
	a.addFlags(f)
	f.StringVar(&a.UnitName, "unit-name", "", "name of the unit to run")
	if err := f.Parse(true, args); err != nil {
		return err
	}
	if a.UnitName == "" {
		return requiredError("unit-name")
	}
	if !juju.ValidUnit.MatchString(a.UnitName) {
		return fmt.Errorf(`--unit-name option expects "<service>/<n>" argument`)
	}
	return a.checkArgs(f.Args())
}

// Run runs a unit agent.
func (a *UnitAgent) Run(_ *cmd.Context) error {
	return fmt.Errorf("UnitAgent.Run not implemented")
}
