package embedded

import _ "embed"

//go:generate cp ../../config.example.yaml .

//go:embed config.example.yaml
var DefaultConfigTemplate []byte
