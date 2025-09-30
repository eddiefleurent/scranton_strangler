module github.com/eddiefleurent/scranton_strangler/scripts/audit_positions

go 1.25.1

replace github.com/eddiefleurent/scranton_strangler => ../..

require github.com/eddiefleurent/scranton_strangler v0.0.0-00010101000000-000000000000

require (
	github.com/sony/gobreaker v1.0.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
