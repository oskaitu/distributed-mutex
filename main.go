package main

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"os"
	"strconv"
	"sync"
	"time"

	beer "github.com/oskaitu/distributed-mutex/grpc"
	"google.golang.org/grpc"
	glog "google.golang.org/grpc/grpclog"
)

var grpcLog glog.LoggerV2

var amount_of_peers = 3

type Peer struct {
	beer.UnimplementedCriticalPingServer
	id           int32
	clients      map[int32]beer.CriticalPingClient
	timeForPeers int32
	ctx          context.Context
	status    	 *State
	
}
type State struct {
	held bool
}

var mutex = &sync.Mutex{}

func init() {
	grpcLog = glog.NewLoggerV2(os.Stdout, os.Stdout, os.Stdout)
}

func main() {
	arg1, _ := strconv.ParseInt(os.Args[1], 10, 32)
	ownPort := int32(arg1) + 5000
	state := &State{
		held: false, 
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	p := &Peer{
		id:           ownPort,
		timeForPeers: 0,
		clients:      make(map[int32]beer.CriticalPingClient),
		ctx:          ctx,
		status: 		  state,
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

	for i := 0; i < amount_of_peers; i++ {
		port := int32(5000) + int32(i)

		if port == ownPort {
			continue
		}

		var conn *grpc.ClientConn
		//grpcLog.Infof("Trying to dial: %v\n", port)
		conn, err := grpc.Dial(fmt.Sprintf(":%v", port), grpc.WithInsecure(), grpc.WithBlock())
		if err != nil {
			grpcLog.Errorf("Could not connect: %s", err)
		}
		defer conn.Close()
		c := beer.NewCriticalPingClient(conn)
		p.clients[port] = c
	}

	for {
		if(!p.status.held){
			grpcLog.Infof("Trying to take the critical function: %v\n", p.id)
			sleepTime(randomNumberGenerator(5, 15))
			p.sendPingToAll()
		}
		
	}
}

// This is for receiving
func (p *Peer) Ping(ctx context.Context, req *beer.Request) (*beer.Reply, error) {

	rep := &beer.Reply{Id: p.id, Timestamp: p.timeForPeers , Status: p.status.held}

	return rep, nil
}

func (p *Peer) CriticalFunction() {
	p.status.held = true
	mutex.Lock()
	grpcLog.Infof("Hello I am doing the critical function %v, %v", p.id, p.timeForPeers)
	
	sleepTime(randomNumberGenerator(5, 10))
	mutex.Unlock()
	p.status.held = false
	p.timeForPeers++

	grpcLog.Infof("I am done with the critical function %v, %v", p.id, p.timeForPeers)

}

// Source for function https://golang.cafe/blog/golang-sleep-random-time.html
func sleepTime(n int) {
	rand.Seed(time.Now().UnixNano())
	time.Sleep(time.Duration(n) * time.Second)
}

func randomNumberGenerator(min int, max int) int {
	n := (rand.Intn(max-min) + min)
	return n
}
func (p *Peer) sendPingToAskIfTheyHoldIt(){
	request := &beer.Request{Id: p.id, Timestamp: p.timeForPeers}
	counter := 0
	for _, client := range p.clients {
		reply, err := client.Ping(p.ctx, request)
		if err != nil {
			grpcLog.Errorln("Cannot connect to client")
		}
		if(!reply.Status){
			counter++
		}
	}
	if(counter == amount_of_peers - 1){
		p.CriticalFunction()
	}
	grpcLog.Infof("Timestamp approved, critical process was already occupied %v", request.Id)
}

// This is to ping the other clients that are connected
func (p *Peer) sendPingToAll() {
	request := &beer.Request{Id: p.id, Timestamp: p.timeForPeers}
	counterForLesserTimestamps := 0
	counterForEqualTimestamps := 0
	for _, client := range p.clients {
		reply, err := client.Ping(p.ctx, request)
		if err != nil {
			grpcLog.Errorln("Cannot connect to client ")
		}
		
		if (request.Timestamp < reply.Timestamp) {
			counterForLesserTimestamps++

		} else if request.Timestamp == reply.Timestamp {
			
				counterForEqualTimestamps++
		}

		if (counterForLesserTimestamps  == amount_of_peers-1 || 
			counterForEqualTimestamps == amount_of_peers -1 || 
			counterForLesserTimestamps == counterForEqualTimestamps) {
			p.sendPingToAskIfTheyHoldIt()	
			
		}
	}
	grpcLog.Infof("Request for Critical Function was denied %v", request.Id)
}
