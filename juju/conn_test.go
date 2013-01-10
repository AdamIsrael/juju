package juju_test

import (
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/dummy"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/trivial"
	"os"
	"path/filepath"
	stdtesting "testing"
)

func Test(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type NewConnSuite struct {
	coretesting.LoggingSuite
}

var _ = Suite(&NewConnSuite{})

func (cs *NewConnSuite) TearDownTest(c *C) {
	dummy.Reset()
	cs.LoggingSuite.TearDownTest(c)
}

func (*NewConnSuite) TestNewConnWithoutAdminSecret(c *C) {
	attrs := map[string]interface{}{
		"name":            "erewhemos",
		"type":            "dummy",
		"state-server":    true,
		"authorized-keys": "i-am-a-key",
		"secret":          "pork",
		"admin-secret":    "really",
		"ca-cert":         coretesting.CACert,
		"ca-private-key":  coretesting.CAKey,
	}
	env, err := environs.NewFromAttrs(attrs)
	c.Assert(err, IsNil)
	err = environs.Bootstrap(env, false, panicWrite)
	c.Assert(err, IsNil)

	delete(attrs, "admin-secret")
	env1, err := environs.NewFromAttrs(attrs)
	c.Assert(err, IsNil)
	conn, err := juju.NewConn(env1)
	c.Check(conn, IsNil)
	c.Assert(err, ErrorMatches, "cannot connect without admin-secret")
}

func (*NewConnSuite) TestNewConnFromName(c *C) {
	home := c.MkDir()
	defer os.Setenv("HOME", os.Getenv("HOME"))
	os.Setenv("HOME", home)
	conn, err := juju.NewConnFromName("")
	c.Assert(conn, IsNil)
	c.Assert(err, ErrorMatches, ".*: no such file or directory")

	if err := os.Mkdir(filepath.Join(home, ".juju"), 0755); err != nil {
		c.Fatal("Could not create directory structure")
	}
	envs := filepath.Join(home, ".juju", "environments.yaml")
	err = ioutil.WriteFile(envs, []byte(`
default:
    erewhemos
environments:
    erewhemos:
        type: dummy
        state-server: true
        authorized-keys: i-am-a-key
        admin-secret: conn-from-name-secret
`), 0644)

	err = ioutil.WriteFile(filepath.Join(home, ".juju", "erewhemos-cert.pem"), []byte(coretesting.CACert), 0600)
	c.Assert(err, IsNil)
	err = ioutil.WriteFile(filepath.Join(home, ".juju", "erewhemos-private-key.pem"), []byte(coretesting.CAKey), 0600)
	c.Assert(err, IsNil)

	// Just run through a few operations on the dummy provider and verify that
	// they behave as expected.
	conn, err = juju.NewConnFromName("")
	c.Assert(err, ErrorMatches, "dummy environment not bootstrapped")

	environ, err := environs.NewFromName("")
	c.Assert(err, IsNil)
	err = environs.Bootstrap(environ, false, panicWrite)
	c.Assert(err, IsNil)

	conn, err = juju.NewConnFromName("")
	c.Assert(err, IsNil)
	defer conn.Close()
	c.Assert(conn.Environ, NotNil)
	c.Assert(conn.Environ.Name(), Equals, "erewhemos")
	c.Assert(conn.State, NotNil)

	// Reset the admin password so the state db can be reused.
	err = conn.State.SetAdminPassword("")
	c.Assert(err, IsNil)
	// Close the conn (thereby closing its state) a couple of times to
	// verify that multiple closes will not panic. We ignore the error,
	// as the underlying State will return an error the second
	// time.
	conn.Close()
	conn.Close()
}

func (cs *NewConnSuite) TestConnStateSecretsSideEffect(c *C) {
	attrs := map[string]interface{}{
		"name":            "erewhemos",
		"type":            "dummy",
		"state-server":    true,
		"authorized-keys": "i-am-a-key",
		"secret":          "pork",
		"admin-secret":    "side-effect secret",
		"ca-cert":         coretesting.CACert,
		"ca-private-key":  coretesting.CAKey,
	}
	env, err := environs.NewFromAttrs(attrs)
	c.Assert(err, IsNil)
	err = environs.Bootstrap(env, false, panicWrite)
	c.Assert(err, IsNil)
	info, err := env.StateInfo()
	c.Assert(err, IsNil)
	info.Password = trivial.PasswordHash("side-effect secret")
	st, err := state.Open(info)
	c.Assert(err, IsNil)

	// Verify we have no secret in the environ config
	cfg, err := st.EnvironConfig()
	c.Assert(err, IsNil)
	c.Assert(cfg.UnknownAttrs()["secret"], IsNil)

	// Make a new Conn, which will push the secrets.
	conn, err := juju.NewConn(env)
	c.Assert(err, IsNil)
	defer conn.Close()

	cfg, err = conn.State.EnvironConfig()
	c.Assert(err, IsNil)
	c.Assert(cfg.UnknownAttrs()["secret"], Equals, "pork")

	// Reset the admin password so the state db can be reused.
	err = conn.State.SetAdminPassword("")
	c.Assert(err, IsNil)
}

func (cs *NewConnSuite) TestConnStateDoesNotUpdateExistingSecrets(c *C) {
	attrs := map[string]interface{}{
		"name":            "erewhemos",
		"type":            "dummy",
		"state-server":    true,
		"authorized-keys": "i-am-a-key",
		"secret":          "pork",
		"admin-secret":    "some secret",
		"ca-cert":         coretesting.CACert,
		"ca-private-key":  coretesting.CAKey,
	}
	env, err := environs.NewFromAttrs(attrs)
	c.Assert(err, IsNil)
	err = environs.Bootstrap(env, false, panicWrite)
	c.Assert(err, IsNil)

	// Make a new Conn, which will push the secrets.
	conn, err := juju.NewConn(env)
	c.Assert(err, IsNil)
	defer conn.Close()

	// Make another env with a different secret.
	attrs["secret"] = "squirrel"
	env1, err := environs.NewFromAttrs(attrs)
	c.Assert(err, IsNil)

	// Connect with the new env and check that the secret has not changed
	conn, err = juju.NewConn(env1)
	c.Assert(err, IsNil)
	defer conn.Close()
	cfg, err := conn.State.EnvironConfig()
	c.Assert(err, IsNil)
	c.Assert(cfg.UnknownAttrs()["secret"], Equals, "pork")

	// Reset the admin password so the state db can be reused.
	err = conn.State.SetAdminPassword("")
	c.Assert(err, IsNil)
}

func panicWrite(name string, cert, key []byte) error {
	panic("writeCertAndKey called unexpectedly")
}

func (cs *NewConnSuite) TestConnWithPassword(c *C) {
	env, err := environs.NewFromAttrs(map[string]interface{}{
		"name":            "erewhemos",
		"type":            "dummy",
		"state-server":    true,
		"authorized-keys": "i-am-a-key",
		"secret":          "squirrel",
		"admin-secret":    "nutkin",
		"ca-cert":         coretesting.CACert,
		"ca-private-key":  coretesting.CAKey,
	})
	c.Assert(err, IsNil)
	err = environs.Bootstrap(env, false, panicWrite)
	c.Assert(err, IsNil)

	// Check that Bootstrap has correctly used a hash
	// of the admin password.
	info, err := env.StateInfo()
	c.Assert(err, IsNil)
	info.Password = trivial.PasswordHash("nutkin")
	st, err := state.Open(info)
	c.Assert(err, IsNil)
	st.Close()

	// Check that we can connect with the original environment.
	conn, err := juju.NewConn(env)
	c.Assert(err, IsNil)
	conn.Close()

	// Check that the password has now been changed to the original
	// admin password.
	info.Password = "nutkin"
	st1, err := state.Open(info)
	c.Assert(err, IsNil)
	st1.Close()

	// Check that we can still connect with the original
	// environment.
	conn, err = juju.NewConn(env)
	c.Assert(err, IsNil)
	defer conn.Close()

	// Reset the admin password so the state db can be reused.
	err = conn.State.SetAdminPassword("")
	c.Assert(err, IsNil)
}

type ConnSuite struct {
	coretesting.LoggingSuite
	coretesting.MgoSuite
	conn *juju.Conn
	repo *charm.LocalRepository
}

var _ = Suite(&ConnSuite{})

func (s *ConnSuite) SetUpTest(c *C) {
	s.LoggingSuite.SetUpTest(c)
	s.MgoSuite.SetUpTest(c)
	attrs := map[string]interface{}{
		"name":            "erewhemos",
		"type":            "dummy",
		"state-server":    true,
		"authorized-keys": "i-am-a-key",
		"admin-secret":    "deploy-test-secret",
		"ca-cert":         coretesting.CACert,
		"ca-private-key":  coretesting.CAKey,
	}
	environ, err := environs.NewFromAttrs(attrs)
	c.Assert(err, IsNil)
	err = environs.Bootstrap(environ, false, panicWrite)
	c.Assert(err, IsNil)
	s.conn, err = juju.NewConn(environ)
	c.Assert(err, IsNil)
	s.repo = &charm.LocalRepository{Path: c.MkDir()}
}

func (s *ConnSuite) TearDownTest(c *C) {
	if s.conn == nil {
		return
	}
	err := s.conn.State.SetAdminPassword("")
	c.Assert(err, IsNil)
	err = s.conn.Environ.Destroy(nil)
	c.Check(err, IsNil)
	s.conn.Close()
	s.conn = nil
	s.MgoSuite.TearDownTest(c)
	s.LoggingSuite.TearDownTest(c)
}

func (s *ConnSuite) SetUpSuite(c *C) {
	s.LoggingSuite.SetUpSuite(c)
	s.MgoSuite.SetUpSuite(c)
}

func (s *ConnSuite) TearDownSuite(c *C) {
	s.LoggingSuite.TearDownSuite(c)
	s.MgoSuite.TearDownSuite(c)
}

func (s *ConnSuite) TestPutCharmBasic(c *C) {
	curl := coretesting.Charms.ClonedURL(s.repo.Path, "series", "riak")
	curl.Revision = -1 // make sure we trigger the repo.Latest logic.
	sch, err := s.conn.PutCharm(curl, s.repo, false)
	c.Assert(err, IsNil)
	c.Assert(sch.Meta().Summary, Equals, "K/V storage engine")

	sch, err = s.conn.State.Charm(sch.URL())
	c.Assert(err, IsNil)
	c.Assert(sch.Meta().Summary, Equals, "K/V storage engine")
}

func (s *ConnSuite) TestPutBundledCharm(c *C) {
	// Bundle the riak charm into a charm repo directory.
	dir := filepath.Join(s.repo.Path, "series")
	err := os.Mkdir(dir, 0777)
	c.Assert(err, IsNil)
	w, err := os.Create(filepath.Join(dir, "riak.charm"))
	c.Assert(err, IsNil)
	defer w.Close()
	charmDir := coretesting.Charms.Dir("series", "riak")
	err = charmDir.BundleTo(w)
	c.Assert(err, IsNil)

	// Invent a URL that points to the bundled charm, and
	// test putting that.
	curl := &charm.URL{
		Schema:   "local",
		Series:   "series",
		Name:     "riak",
		Revision: -1,
	}
	sch, err := s.conn.PutCharm(curl, s.repo, false)
	c.Assert(err, IsNil)
	c.Assert(sch.Meta().Summary, Equals, "K/V storage engine")

	// Check that we can get the charm from the state.
	sch, err = s.conn.State.Charm(sch.URL())
	c.Assert(err, IsNil)
	c.Assert(sch.Meta().Summary, Equals, "K/V storage engine")
}

func (s *ConnSuite) TestPutCharmUpload(c *C) {
	repo := &charm.LocalRepository{c.MkDir()}
	curl := coretesting.Charms.ClonedURL(repo.Path, "series", "riak")

	// Put charm for the first time.
	sch, err := s.conn.PutCharm(curl, repo, false)
	c.Assert(err, IsNil)
	c.Assert(sch.Meta().Summary, Equals, "K/V storage engine")

	sch, err = s.conn.State.Charm(sch.URL())
	c.Assert(err, IsNil)
	sha256 := sch.BundleSha256()
	rev := sch.Revision()

	// Change the charm on disk.
	ch, err := repo.Get(curl)
	c.Assert(err, IsNil)
	chd := ch.(*charm.Dir)
	err = ioutil.WriteFile(filepath.Join(chd.Path, "extra"), []byte("arble"), 0666)
	c.Assert(err, IsNil)

	// Put charm again and check that it has not changed in the state.
	sch, err = s.conn.PutCharm(curl, repo, false)
	c.Assert(err, IsNil)

	sch, err = s.conn.State.Charm(sch.URL())
	c.Assert(err, IsNil)
	c.Assert(sch.BundleSha256(), Equals, sha256)
	c.Assert(sch.Revision(), Equals, rev)

	// Put charm again, with bumpRevision this time, and check that
	// it has changed.
	sch, err = s.conn.PutCharm(curl, repo, true)
	c.Assert(err, IsNil)

	sch, err = s.conn.State.Charm(sch.URL())
	c.Assert(err, IsNil)
	c.Assert(sch.BundleSha256(), Not(Equals), sha256)
	c.Assert(sch.Revision(), Equals, rev+1)
}

func (s *ConnSuite) TestAddService(c *C) {
	curl := coretesting.Charms.ClonedURL(s.repo.Path, "series", "riak")
	sch, err := s.conn.PutCharm(curl, s.repo, false)
	c.Assert(err, IsNil)

	svc, err := s.conn.AddService("testriak", sch)
	c.Assert(err, IsNil)

	// Check that the peer relation has been made.
	relations, err := svc.Relations()
	c.Assert(relations, HasLen, 1)
	ep, err := relations[0].Endpoint("testriak")
	c.Assert(err, IsNil)
	c.Assert(ep, Equals, state.Endpoint{
		ServiceName:   "testriak",
		Interface:     "riak",
		RelationName:  "ring",
		RelationRole:  state.RolePeer,
		RelationScope: charm.ScopeGlobal,
	})
}

func (s *ConnSuite) TestAddServiceDefaultName(c *C) {
	curl := coretesting.Charms.ClonedURL(s.repo.Path, "series", "riak")
	sch, err := s.conn.PutCharm(curl, s.repo, false)
	c.Assert(err, IsNil)

	svc, err := s.conn.AddService("", sch)
	c.Assert(err, IsNil)
	c.Assert(svc.Name(), Equals, "riak")
}

func (s *ConnSuite) TestAddUnits(c *C) {
	curl := coretesting.Charms.ClonedURL(s.repo.Path, "series", "riak")
	sch, err := s.conn.PutCharm(curl, s.repo, false)
	c.Assert(err, IsNil)
	svc, err := s.conn.AddService("testriak", sch)
	c.Assert(err, IsNil)
	units, err := s.conn.AddUnits(svc, 2)
	c.Assert(err, IsNil)
	c.Assert(units, HasLen, 2)

	id0, err := units[0].AssignedMachineId()
	c.Assert(err, IsNil)
	id1, err := units[1].AssignedMachineId()
	c.Assert(err, IsNil)
	c.Assert(id0, Not(Equals), id1)
}

func (s *ConnSuite) TestDestroyPrincipalUnits(c *C) {
	// Create 3 principal units.
	curl := coretesting.Charms.ClonedURL(s.repo.Path, "series", "wordpress")
	sch, err := s.conn.PutCharm(curl, s.repo, false)
	wordpress, err := s.conn.AddService("wordpress", sch)
	c.Assert(err, IsNil)
	for i := 0; i < 3; i++ {
		_, err = wordpress.AddUnit()
		c.Assert(err, IsNil)
	}

	// Destroy 2 of them; check they become Dying.
	err = s.conn.DestroyUnits("wordpress/0", "wordpress/1")
	c.Assert(err, IsNil)
	s.assertUnitLife(c, "wordpress/0", state.Dying)
	s.assertUnitLife(c, "wordpress/1", state.Dying)

	// Try to destroy the remaining one along with a pre-destroyed one; check
	// it fails.
	err = s.conn.DestroyUnits("wordpress/2", "wordpress/0")
	c.Assert(err, ErrorMatches, `cannot destroy units: unit "wordpress/0" is not alive`)
	s.assertUnitLife(c, "wordpress/2", state.Alive)

	// Try to destroy the remaining one along with a nonexistent one; check it
	// fails.
	err = s.conn.DestroyUnits("wordpress/2", "boojum/123")
	c.Assert(err, ErrorMatches, `cannot destroy units: unit "boojum/123" is not alive`)
	s.assertUnitLife(c, "wordpress/2", state.Alive)

	// Destroy the remaining unit on its own, accidentally specifying it twice;
	// this should work.
	err = s.conn.DestroyUnits("wordpress/2", "wordpress/2")
	c.Assert(err, IsNil)
	s.assertUnitLife(c, "wordpress/2", state.Dying)
}

func (s *ConnSuite) TestDestroySubordinateUnits(c *C) {
	// Create a principal and a subordinate.
	wpcurl := coretesting.Charms.ClonedURL(s.repo.Path, "series", "wordpress")
	wpsch, err := s.conn.PutCharm(wpcurl, s.repo, false)
	wordpress, err := s.conn.AddService("wordpress", wpsch)
	c.Assert(err, IsNil)
	wordpress0, err := wordpress.AddUnit()
	c.Assert(err, IsNil)
	err = wordpress0.SetPrivateAddress("meh")
	c.Assert(err, IsNil)
	lgcurl := coretesting.Charms.ClonedURL(s.repo.Path, "series", "logging")
	lgsch, err := s.conn.PutCharm(lgcurl, s.repo, false)
	_, err = s.conn.AddService("logging", lgsch)
	c.Assert(err, IsNil)
	eps, err := s.conn.State.InferEndpoints([]string{"logging", "wordpress"})
	c.Assert(err, IsNil)
	rel, err := s.conn.State.AddRelation(eps...)
	c.Assert(err, IsNil)
	ru, err := rel.Unit(wordpress0)
	c.Assert(err, IsNil)
	err = ru.EnterScope()
	c.Assert(err, IsNil)

	// Try to destroy the subordinate alone; check it fails.
	err = s.conn.DestroyUnits("logging/0")
	c.Assert(err, ErrorMatches, `cannot destroy units: unit "logging/0" is a subordinate`)
	s.assertUnitLife(c, "logging/0", state.Alive)

	// Try to destroy the principal and the subordinate together; check it fails.
	err = s.conn.DestroyUnits("wordpress/0", "logging/0")
	c.Assert(err, ErrorMatches, `cannot destroy units: unit "logging/0" is a subordinate`)
	s.assertUnitLife(c, "wordpress/0", state.Alive)
	s.assertUnitLife(c, "logging/0", state.Alive)

	// Destroy the principal; check the subordinate does not become Dying. (This
	// is the unit agent's responsibility.)
	err = s.conn.DestroyUnits("wordpress/0")
	c.Assert(err, IsNil)
	s.assertUnitLife(c, "wordpress/0", state.Dying)
	s.assertUnitLife(c, "logging/0", state.Alive)
}

func (s *ConnSuite) assertUnitLife(c *C, name string, life state.Life) {
	unit, err := s.conn.State.Unit(name)
	c.Assert(err, IsNil)
	c.Assert(unit.Refresh(), IsNil)
	c.Assert(unit.Life(), Equals, life)
}
func (s *ConnSuite) TestResolved(c *C) {
	curl := coretesting.Charms.ClonedURL(s.repo.Path, "series", "riak")
	sch, err := s.conn.PutCharm(curl, s.repo, false)
	c.Assert(err, IsNil)
	svc, err := s.conn.AddService("testriak", sch)
	c.Assert(err, IsNil)
	us, err := s.conn.AddUnits(svc, 1)
	c.Assert(err, IsNil)
	u := us[0]

	err = s.conn.Resolved(u, false)
	c.Assert(err, ErrorMatches, `unit "testriak/0" is not in an error state`)
	err = s.conn.Resolved(u, true)
	c.Assert(err, ErrorMatches, `unit "testriak/0" is not in an error state`)

	err = u.SetStatus(state.UnitError, "gaaah")
	c.Assert(err, IsNil)
	err = s.conn.Resolved(u, false)
	c.Assert(err, IsNil)
	err = s.conn.Resolved(u, true)
	c.Assert(err, ErrorMatches, `cannot set resolved mode for unit "testriak/0": already resolved`)
	c.Assert(u.Resolved(), Equals, state.ResolvedNoHooks)

	err = u.ClearResolved()
	c.Assert(err, IsNil)
	err = s.conn.Resolved(u, true)
	c.Assert(err, IsNil)
	err = s.conn.Resolved(u, false)
	c.Assert(err, ErrorMatches, `cannot set resolved mode for unit "testriak/0": already resolved`)
	c.Assert(u.Resolved(), Equals, state.ResolvedRetryHooks)
}
