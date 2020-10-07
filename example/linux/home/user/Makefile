CMDS=myscript myscript2

all: $(CMDS)
.PHONY: $(CMDS)

$(CMDS):
# run gorun twice to make sure the first -embed extracts go.mod/go.sum if it needs to
	gorun -embed $@/*.go
	(cd $@; go mod tidy; gofmt -w $@.go; go test ./...)
	gorun -embed $@/*.go
