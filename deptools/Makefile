gx_version=v0.14.1
gx-go_version=v1.9.0
gateway=https://ipfs.io
local_gateway=http://127.0.0.1:8080
dist=dist.ipfs.io
dl_cmd=wget -nc

gx=gx_$(gx_version)
gx-go=gx-go_$(gx-go_version)
bin_env=$(shell go env GOHOSTOS)-$(shell go env GOHOSTARCH)

gx_tar=v0.14.1.tar.gz
gx-go_tar=v1.9.0.tar.gz

# gx_dist_path=/ipns/$(dist)/gx/$(gx_version)/$(gx_tar)
# gx-go_dist_path=/ipns/$(dist)/gx-go/$(gx-go_version)/$(gx-go_tar)

# gx_download_local=$(dl_cmd) $(local_gateway)$(gx_dist_path)
# gx_download=$(dl_cmd) $(gateway)$(gx_dist_path)

# gx-go_download_local=$(dl_cmd) $(local_gateway)$(gx-go_dist_path)
# gx-go_download=$(dl_cmd) $(gateway)$(gx-go_dist_path)

gx_download=https://github.com/whyrusleeping/gx/archive/v0.14.1.tar.gz
gx-go_download=https://github.com/whyrusleeping/gx-go/archive/v1.9.0.tar.gz

$(gx):
	@echo "Downloading gx"
	rm -f gx
	$(dl_cmd) $(gx_download)
	tar xvzf $(gx_tar)
	mv gx-0.14.1 $(gx)
	ln -s $(gx) gx
	rm $(gx_tar)

$(gx-go):
	@echo "Downloading gx-go"
	rm -f gx-go
	$(dl_cmd) $(gx-go_download)
	tar xvzf $(gx-go_tar) 
	mv gx-go-1.9.0 $(gx-go)
	ln -s $(gx-go) gx-go
	rm $(gx-go_tar)

gx: $(gx) $(gx-go)

gx-clean:
	@rm -f gx*
