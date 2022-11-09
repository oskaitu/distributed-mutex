package main

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"strconv"

	beer "github.com/oskaitu/distributed-mutex/grpc"
	"google.golang.org/grpc"
	glog "google.golang.org/grpc/grpclog"
)

var grpcLog glog.LoggerV2

type Peer struct {
	beer.UnimplementedCriticalPingServer
	id      int32
	clients map[int32]beer.CriticalPingClient
	ctx     context.Context
}

func init() {
	grpcLog = glog.NewLoggerV2(os.Stdout, os.Stdout, os.Stdout)
}

func main() {
	arg1, _ := strconv.ParseInt(os.Args[1], 10, 32)
	ownPort := int32(arg1) + 5000

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	p := &Peer{
		id:      ownPort,
		clients: make(map[int32]beer.CriticalPingClient),
		ctx:     ctx,
	}

	// Create listener tcp on port ownPort
	list, err := net.Listen("tcp", fmt.Sprintf(":%v", ownPort))
	if err != nil {
		grpcLog.Errorf("Failed to listen on port: %v", err)
	}
	grpcServer := grpc.NewServer()
	beer.RegisterCriticalPingServer(grpcServer, p)

	go func() {
		if err := grpcServer.Serve(list); err != nil {
			grpcLog.Errorf("failed to server %v", err)
		}
	}()

	for i := 0; i < 3; i++ {
		port := int32(5000) + int32(i)

		if port == ownPort {
			continue
		}

		var conn *grpc.ClientConn
		grpcLog.Infof("Trying to dial: %v\n", port)
		conn, err := grpc.Dial(fmt.Sprintf(":%v", port), grpc.WithInsecure(), grpc.WithBlock())
		if err != nil {
			grpcLog.Errorf("Could not connect: %s", err)
		}
		defer conn.Close()
		c := beer.NewCriticalPingClient(conn)
		p.clients[port] = c
	}

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		p.sendPingToAll()
	}
}

// This is for receiving
func (p *Peer) Ping(ctx context.Context, req *beer.Request) (*beer.Reply, error) {
	id := req.Id

	grpcLog.Infof("Räsciv ping from %v\n", id)
	rep := &beer.Reply{Id: p.id}

	return rep, nil
}

// This is to ping the other clients that are connected
func (p *Peer) sendPingToAll() {
	request := &beer.Request{Id: p.id}
	for id, client := range p.clients {
		reply, err := client.Ping(p.ctx, request)
		if err != nil {
			grpcLog.Errorln("Cannot connect to client")
		}
		grpcLog.Infof("Gor ping reply back from id %v : %v\n--------------\n", id, reply.Id)
		
	}
}
