gx_version=v0.14.1
gx-go_version=v1.9.0
gateway=https://ipfs.io
gateway_ipfsboost=https://dist.ipfsboost.com
local_gateway=http://127.0.0.1:8080
dist=dist.ipfs.io
dl_cmd=wget -nc

gx=gx_$(gx_version)
gx-go=gx-go_$(gx-go_version)
bin_env=$(shell go env GOHOSTOS)-$(shell go env GOHOSTARCH)

gx_tar=$(gx)_$(bin_env).tar.gz
gx-go_tar=$(gx-go)_$(bin_env).tar.gz

gx_dist_path=/ipns/$(dist)/gx/$(gx_version)/$(gx_tar)
gx-go_dist_path=/ipns/$(dist)/gx-go/$(gx-go_version)/$(gx-go_tar)

gx_download_local=$(dl_cmd) $(local_gateway)$(gx_dist_path)
gx_download=$(dl_cmd) $(gateway)$(gx_dist_path)
gx_download_ipfsboost=$(dl_cmd) $(gateway_ipfsboost)$(gx_dist_path)

gx-go_download_local=$(dl_cmd) $(local_gateway)$(gx-go_dist_path)
gx-go_download=$(dl_cmd) $(gateway)$(gx-go_dist_path)
gx-go_download_ipfsboost=$(dl_cmd) $(gateway_ipfsboost)$(gx-go_dist_path)

$(gx):
	@echo "Downloading gx"
	rm -f gx
	$(gx_download_local) || $(gx_download_ipfsboost) || $(gx_download)
	tar -zxf $(gx_tar) --strip-components=1 gx/gx
	mv gx $(gx)
	ln -s $(gx) gx
	rm $(gx_tar)

$(gx-go):
	@echo "Downloading gx-go"
	rm -f gx-go
	$(gx-go_download_local) || $(gx-go_download_ipfsboost) || $(gx-go_download)
	tar -zxf $(gx-go_tar) --strip-components=1 gx-go/gx-go
	mv gx-go $(gx-go)
	ln -s $(gx-go) gx-go
	rm $(gx-go_tar)

gx: $(gx) $(gx-go)

gx-clean:
	@rm -f gx*
