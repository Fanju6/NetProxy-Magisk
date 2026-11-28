module github.com/Fanju6/NetProxy-Magisk/src/native/netproxy

go 1.27.0

require (
	github.com/fsnotify/fsnotify v1.10.1
	github.com/mdlayher/genetlink v1.4.0
	github.com/mdlayher/wifi v0.9.0
	github.com/sagernet/bbolt v0.0.0-20261124060517-2ab7bd6d2bff
	github.com/sagernet/netlink v0.0.0-20261125120807-b006b90aa077
	github.com/sagernet/sing v0.9.7-0.20261127131308-bbcd8ecc42a3
	github.com/sagernet/sing-box v1.15.0-alpha.15-reF1nd
	github.com/sagernet/sing-tun v0.9.7-0.20261126024559-1a13df1ac21f
	golang.org/x/sys v0.50.0
	google.golang.org/protobuf v1.36.12
	gopkg.in/yaml.v3 v3.0.1
)

replace github.com/sagernet/sing-box => github.com/reF1nd/sing-box v1.15.0-alpha.15-reF1nd

require (
	github.com/florianl/go-nfqueue/v2 v2.1.0 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/gofrs/uuid/v5 v5.5.1 // indirect
	github.com/google/btree v1.1.3 // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/logrusorgru/aurora v2.0.3+incompatible // indirect
	github.com/mdlayher/netlink v1.11.2 // indirect
	github.com/mdlayher/socket v0.7.0 // indirect
	github.com/miekg/dns v1.1.72 // indirect
	github.com/sagernet/fswatch v0.1.2 // indirect
	github.com/sagernet/gvisor v0.0.0-20260727.0-sing-box-mod.1 // indirect
	github.com/sagernet/nftables v0.3.0-mod.4.0.20261114074246-0b310c3e010b // indirect
	github.com/sagernet/sing-usbip v0.0.0-20260817040617-28bd42667eca // indirect
	github.com/sagernet/sing-vmess v0.2.9-0.20261124075649-4acc92a6f6c6 // indirect
	github.com/vishvananda/netns v0.0.5 // indirect
	go4.org/netipx v0.0.0-20260823151212-3075585bcbeb // indirect
	golang.org/x/crypto v0.59.0 // indirect
	golang.org/x/exp v0.0.0-20261124000051-d6646c509b6d // indirect
	golang.org/x/mod v0.43.0 // indirect
	golang.org/x/net v0.63.0 // indirect
	golang.org/x/sync v0.25.0 // indirect
	golang.org/x/time v0.16.0 // indirect
	golang.org/x/tools v0.53.0 // indirect
)
