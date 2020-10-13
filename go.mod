module github.com/mazzy89/containervmm

go 1.14

replace (
	github.com/kata-containers/govmm => github.com/mazzy89/govmm v0.0.0-fix-fwcfg
	github.com/vishvananda/netlink => github.com/twelho/netlink v1.1.1-ageing
)

require (
	code.cloudfoundry.org/bytefmt v0.0.0-20200131002437-cf55d5288a48
	github.com/kata-containers/govmm v0.0.0-20201009140431-546cc55ea419
	github.com/krolaw/dhcp4 v0.0.0-20190909130307-a50d88189771
	github.com/miekg/dns v1.1.31
	github.com/onsi/ginkgo v1.14.1 // indirect
	github.com/onsi/gomega v1.10.2 // indirect
	github.com/schollz/progressbar/v3 v3.6.0
	github.com/sirupsen/logrus v1.7.0
	github.com/spf13/pflag v1.0.5
	github.com/vishvananda/netlink v1.1.0
	golang.org/x/crypto v0.0.0-20201002170205-7f63de1d35b0
)
