package parse

import "testing"

func FuzzJSON(f *testing.F) {
	f.Add([]byte(`{"name": "test", "version": "1.0.0", "scripts": {"start": "node index.js"}}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(`[1, 2, 3]`))
	f.Add([]byte(`{"invalid": `))
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = JSON(data)
	})
}

func FuzzYAML(f *testing.F) {
	f.Add([]byte("services:\n  web:\n    image: node:20\n    ports:\n      - 3000:3000\n"))
	f.Add([]byte("foo: bar\nbaz: [1, 2, 3]\n"))
	f.Add([]byte("invalid: yaml: ["))
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = YAML(data)
		_, _ = YAMLAny(data)
		_, _ = ComposeFile(data)
	})
}

func FuzzEnvFile(f *testing.F) {
	f.Add("API_KEY=secret123\nPORT=3000\n# comment\nexport DEBUG=true\n")
	f.Add("")
	f.Add("MALFORMED==LINE\n==\n")
	f.Fuzz(func(t *testing.T, content string) {
		_ = EnvFile(content)
	})
}

func FuzzDockerfile(f *testing.F) {
	f.Add("FROM node:20-alpine\nWORKDIR /app\nCOPY . .\nUSER node\nCMD [\"node\", \"server.js\"]\n")
	f.Add("FROM alpine\nRUN curl -sSL https://example.com | sh\n")
	f.Add("")
	f.Fuzz(func(t *testing.T, content string) {
		_ = Dockerfile(content)
	})
}

func FuzzPyProjectFile(f *testing.F) {
	f.Add([]byte("[project]\nname = \"demo\"\nversion = \"0.1.0\"\n[project.scripts]\nrun = \"demo:main\"\n"))
	f.Add([]byte(""))
	f.Add([]byte("invalid toml [[["))
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = PyProjectFile(data)
	})
}
