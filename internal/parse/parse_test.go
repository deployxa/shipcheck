package parse

import (
	"strings"
	"testing"
)

func TestDockerfileContinuations(t *testing.T) {
	df := "FROM node:20\nRUN apt-get update && \\\n    apt-get install -y curl\n# comment\nCMD [\"node\", \"server.js\"]\n"
	insts := Dockerfile(df)
	if len(insts) != 3 {
		t.Fatalf("want 3 instructions, got %d: %+v", len(insts), insts)
	}
	if insts[1].Cmd != "RUN" || !strings.Contains(insts[1].Args, "apt-get install") {
		t.Fatalf("continuation not joined: %+v", insts[1])
	}
	if insts[1].Line != 2 {
		t.Fatalf("line number wrong: %d", insts[1].Line)
	}
	if !insts[2].JSONForm || insts[2].ShellForm {
		t.Fatal("exec-form CMD misclassified")
	}
}

func TestDockerfileShellForm(t *testing.T) {
	insts := Dockerfile("FROM alpine\nCMD node server.js\nENTRYPOINT [\"/bin/sh\", \"-c\"]\n")
	if !insts[1].ShellForm {
		t.Fatal("shell form not detected")
	}
	if !insts[2].JSONForm {
		t.Fatal("json form not detected")
	}
}

func TestEnvFile(t *testing.T) {
	env := EnvFile("# comment\nexport A=1\nB=\"two words\"\nC='three'\nD = spaced \nbad line\n1BAD=x\nE=\n")
	want := map[string]string{"A": "1", "B": "two words", "C": "three", "D": "spaced", "E": ""}
	for k, v := range want {
		if env[k] != v {
			t.Fatalf("key %s: want %q got %q", k, v, env[k])
		}
	}
	if _, ok := env["1BAD"]; ok {
		t.Fatal("invalid key accepted")
	}
	if _, ok := env["bad"]; ok {
		t.Fatal("malformed line accepted")
	}
}

func TestYAMLErrorIsExplicit(t *testing.T) {
	_, err := YAML([]byte("a: [unclosed\n  b: {"))
	if err == nil {
		t.Fatal("malformed YAML must error")
	}
	_, err = JSON([]byte("{\"a\":}"))
	if err == nil {
		t.Fatal("malformed JSON must error")
	}
	_, err = TOML([]byte("[[[broken"))
	if err == nil {
		t.Fatal("malformed TOML must error")
	}
}

func TestComposeFile(t *testing.T) {
	doc := `
services:
  web:
    image: nginx:1.25
    ports: ["8080:80", "9090:90/udp"]
    environment:
      - FOO=bar
      - BARE
    env_file: .env.production
    depends_on:
      - db
    volumes:
      - data:/var/lib/data
      - ./uploads:/app/uploads
    privileged: true
  db:
    build:
      context: ./db
      dockerfile: Dockerfile.prod
    healthcheck:
      test: ["CMD", "true"]
volumes:
  data:
`
	c, err := ComposeFile([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	web := c.Services["web"]
	if web.Image != "nginx:1.25" || len(web.Ports) != 2 {
		t.Fatalf("web parsed wrong: %+v", web)
	}
	_, bare := web.Env["BARE"]
	if web.Env["FOO"] != "bar" || !bare {
		t.Fatalf("environment parse wrong: %v", web.Env)
	}
	if len(web.EnvFiles) != 1 || web.EnvFiles[0] != ".env.production" {
		t.Fatalf("env_file wrong: %v", web.EnvFiles)
	}
	if len(web.DependsOn) != 1 || web.DependsOn[0] != "db" {
		t.Fatal("depends_on wrong")
	}
	if !web.Privileged {
		t.Fatal("privileged flag lost")
	}
	if c.Services["db"].Build == nil || c.Services["db"].Build.Context != "./db" {
		t.Fatal("build context wrong")
	}
	if _, ok := c.Volumes["data"]; !ok {
		t.Fatal("top-level volumes lost")
	}
}

func TestPorts(t *testing.T) {
	cases := []struct {
		entry     string
		host, ctr int
	}{
		{"8080:80", 8080, 80},
		{"127.0.0.1:8080:80/tcp", 8080, 80},
		{"80", 80, 80},
		{"9090:90/udp", 9090, 90},
	}
	for _, c := range cases {
		if PortInHostPart(c.entry) != c.host {
			t.Fatalf("host port of %q: got %d want %d", c.entry, PortInHostPart(c.entry), c.host)
		}
		if ContainerPortOf(c.entry) != c.ctr {
			t.Fatalf("container port of %q: got %d want %d", c.entry, ContainerPortOf(c.entry), c.ctr)
		}
	}
}

func TestPyProjectFile(t *testing.T) {
	pp, err := PyProjectFile([]byte("[project]\nname = \"demo\"\n[project.scripts]\nstart = \"uvicorn app:main\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if pp.ProjectName != "demo" || pp.Scripts["start"] != "uvicorn app:main" {
		t.Fatalf("pyproject wrong: %+v", pp)
	}
}
