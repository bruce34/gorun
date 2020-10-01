CMDS=myscript myscript2

all: $(CMDS)
.PHONY: $(CMDS)

$(CMDS):
	(cd $@; go mod tidy; gofmt -w $@.go)
	gorun -embed $@/*.go
