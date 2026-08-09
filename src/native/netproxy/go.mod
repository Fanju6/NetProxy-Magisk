module github.com/Fanju6/NetProxy-Magisk/src/native/netproxy

go 1.26.5

require (
	github.com/sagernet/sing v0.9.0-beta.1
	github.com/sagernet/sing-box v1.14.0-beta.12-reF1nd
	google.golang.org/protobuf v1.36.11
	gopkg.in/yaml.v3 v3.0.1
)

replace github.com/sagernet/sing-box => github.com/reF1nd/sing-box v1.14.0-beta.12-reF1nd

require (
	github.com/miekg/dns v1.1.72 // indirect
	go4.org/netipx v0.0.0-20231129151722-fdeea329fbba // indirect
	golang.org/x/mod v0.37.0 // indirect
	golang.org/x/net v0.57.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/tools v0.47.0 // indirect
)
