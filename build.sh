protoc --proto_path ../../../ -I=./proto --go_out=plugins=grpc:./proto proto/buildserver.proto
mv proto/github.com/brotherlogic/buildserver/proto/* ./
