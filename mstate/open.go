package mstate

import (
	"errors"
	"strings"
	"fmt"

	"labix.org/v2/mgo"
	"labix.org/v2/mgo/txn"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/mstate/presence"
	"launchpad.net/juju-core/mstate/watcher"
)

// Info encapsulates information about cluster of
// servers holding juju state and can be used to make a
// connection to that cluster.
type Info struct {
	// Addrs gives the addresses of the MongoDB servers for the state. 
	// Each address should be in the form address:port.
	Addrs []string

	// UseSSH specifies whether MongoDB should be contacted through an 
	// SSH tunnel.
	UseSSH bool
}

// Open connects to the server described by the given
// info, waits for it to be initialized, and returns a new State
// representing the environment connected to.
func Open(info *Info) (*State, error) {
	st, err := open(info)
	if err != nil {
		return nil, err
	}
	log.Printf("mstate: waiting for state to be initialized")
	// TODO(dfc) wait for the /environment key 
	return st, err
}

func open(info *Info) (*State, error) {
	log.Printf("mstate: opening state; mongo addresses: %q", info.Addrs)
	if len(info.Addrs) == 0 {
		return nil, errors.New("no mongo addresses")
	}
	if !info.UseSSH {
		return Dial(strings.Join(info.Addrs, ","))
	}
	if len(info.Addrs) > 1 {
		return nil, errors.New("ssh connect does not support multiple addresses")
	}
	fwd, session, err := sshDial(info.Addrs[0], "")
	if err != nil {
		return nil, err
	}
	return newState(session, fwd)
}

var indexes = []mgo.Index{
	{Key: []string{"endpoints.relationname"}},
	{Key: []string{"endpoints.servicename"}},
}

// The capped collection used for transaction logs defaults to 200MB.
// It's tweaked in export_test.go to 1MB to avoid the overhead of
// creating and deleting the large file repeatedly.
var (
	logSize      = 200000000
	logSizeTests = 1000000
)

func Dial(servers string) (*State, error) {
	log.Printf("opening state with servers: %q", servers)
	session, err := mgo.Dial(servers)
	if err != nil {
		return nil, err
	}
	return newState(session, nil)
}

func newState(session *mgo.Session, fwd *sshForwarder) (*State, error) {
	db := session.DB("juju")
	pdb := session.DB("presence")
	st := &State{
		db:        db,
		charms:    db.C("charms"),
		machines:  db.C("machines"),
		relations: db.C("relations"),
		services:  db.C("services"),
		settings:  db.C("settings"),
		units:     db.C("units"),
		runner:    txn.NewRunner(txns),
		presence:  pdb.C("presence"),
		fwd:       fwd,
	}
	log := db.C("txns.log")
	info := mgo.CollectionInfo{Capped: true, MaxBytes: logSize}
	// The lack of error code for this error was reported upstream:
	//     https://jira.mongodb.org/browse/SERVER-6992
	if err := log.Create(&info); err != nil && err.Error() != "collection already exists" {
		return nil, fmt.Errorf("cannot create log collection: %v", err)
	}
	st.runner = txn.NewRunner(db.C("txns"))
	st.runner.ChangeLog(db.C("txns.log"))
	st.watcher = watcher.New(db.C("txns.log"))
	st.pwatcher = presence.NewWatcher(pdb.C("presence"))
	for _, index := range indexes {
		if err := st.relations.EnsureIndex(index); err != nil {
			return nil, fmt.Errorf("cannot create database index: %v", err)
		}
	}
	return st, nil
}

func (st *State) Close() error {
	err1 := st.watcher.Stop()
	err2 := st.pwatcher.Stop()
	st.db.Session.Close()
	for _, err := range []error{err1, err2} {
		if err != nil {
			return err
		}
	}
	return nil
}
