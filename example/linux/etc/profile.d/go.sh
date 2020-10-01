# Add go to the PATH. Every user gets root's go binaries on their path
# We might be /bin/sh or /bin/bash. /bin/sh doesn't have $UID, sh uses single '='
user=`id -u -n`
if [ "$user" = "root" ]; then
  export GOPATH=/usr/local/gopath
  export PATH=$PATH:/usr/local/go/bin:/usr/local/gopath/bin
  if [ -z "$HOME" ]; then
    export HOME=/root
  fi
else
  export GOPATH=$HOME/go
  export PATH=$PATH:/usr/local/go/bin:/usr/local/gopath/bin:$GOPATH/bin
fi
