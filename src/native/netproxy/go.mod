module github.com/Fanju6/NetProxy-Magisk/src/native/netproxy

go 1.27.0

require (
	github.com/sagernet/netlink v0.0.0-20260814022025-64455d367bbf
	github.com/sagernet/sing v0.9.1-0.20260904133552-ffcabb706b1c
	github.com/sagernet/sing-box v1.15.0-alpha.2-reF1nd
	golang.org/x/sys v0.47.0
	google.golang.org/protobuf v1.36.11
	gopkg.in/yaml.v3 v3.0.1
)

replace github.com/sagernet/sing-box => github.com/reF1nd/sing-box v1.15.0-alpha.2-reF1nd

require (
	github.com/miekg/dns v1.1.72 // indirect
	github.com/vishvananda/netns v0.0.5 // indirect
	go4.org/netipx v0.0.0-20231129151722-fdeea329fbba // indirect
	golang.org/x/mod v0.37.0 // indirect
	golang.org/x/net v0.57.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/tools v0.47.0 // indirect
)
