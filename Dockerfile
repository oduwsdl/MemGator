FROM ibnesayeed/busybox-certs
MAINTAINER Sawood Alam <ibnesayeed@gmail.com>

RUN wget -q -O /memgator https://github.com/oduwsdl/memgator/releases/download/1.0-rc2/memgator-linux-amd64 && chmod +x /memgator
# Once non-pre release is out, this line can be used to build docker image for the latest release
# RUN wget -q -O - https://api.github.com/repos/oduwsdl/memgator/releases/latest | grep "browser_download_url" | grep "memgator-linux-amd64" | cut -d'"' -f 4 | wget -q -i - -O /memgator && chmod +x /memgator

ENTRYPOINT ["/memgator"]
