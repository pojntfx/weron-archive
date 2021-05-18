// +heroku install ./cmd/weron-signaler
// +heroku goVersion go1.16

module github.com/pojntfx/weron

go 1.16

require (
	github.com/mdlayher/ethernet v0.0.0-20190606142754-0394541c37b7
	github.com/pion/webrtc/v3 v3.0.29
	github.com/songgao/water v0.0.0-20200317203138-2b4b6d7c09d8
	github.com/vishvananda/netlink v1.1.0
	nhooyr.io/websocket v1.8.7
)
