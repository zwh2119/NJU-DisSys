package raft

//
// this is an outline of the API that raft must expose to
// the service (or tester). see comments below for
// each of these functions for more details.
//
// rf = Make(...)
//   create a new Raft server.
// rf.Start(command interface{}) (index, Term, isleader)
//   start agreement on a new log entry
// rf.GetState() (Term, isLeader)
//   ask a Raft for its current Term, and whether it thinks it is leader
// ApplyMsg
//   each time a new entry is committed to the log, each Raft peer
//   should send an ApplyMsg to the service (or tester)
//   in the same server.
//

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"time"
)
import "dissys/labrpc"

// import "bytes"
// import "encoding/gob"

const (
	LEADER = iota
	FOLLOWER
	CANDIDATE
)

const (
	HeartBeatInterval  = 100
	ElectionTimeoutMin = 100
	ElectionTimeoutMax = 500
)

const (
	BackOff = -100
)

// as each Raft peer becomes aware that successive log Entries are
// committed, the peer should send an ApplyMsg to the service (or
// tester) on the same server, via the applyCh passed to Make().
type ApplyMsg struct {
	Index       int
	Command     interface{}
	UseSnapshot bool   // ignore for lab2; only used in lab3
	Snapshot    []byte // ignore for lab2; only used in lab3
}

type LogEntry struct {
	Term    int
	Command interface{}
	Index   int
}

// A Go object implementing a single Raft peer.
type Raft struct {
	mu        sync.Mutex
	peers     []*labrpc.ClientEnd
	persister *Persister
	me        int // index into peers[]

	// Your data here.
	// Look at the paper's Figure 2 for a description of what
	// state a Raft server must maintain.

	currentTerm int
	votedFor    int
	log         []LogEntry

	commitIndex int
	lastApplied int

	nextIndex  []int
	matchIndex []int

	applyChan      chan ApplyMsg
	role           int
	electionTimer  *time.Timer
	heartbeatTimer *time.Timer
	leaderID       int
}

// return currentTerm and whether this server
// believes it is the leader.
func (rf *Raft) GetState() (int, bool) {
	var term int
	var isLeader bool
	// Your code here.
	rf.mu.Lock()
	term = rf.currentTerm
	isLeader = rf.role == LEADER
	rf.mu.Unlock()
	return term, isLeader
}

// save Raft's persistent state to stable storage,
// where it can later be retrieved after a crash and restart.
// see paper's Figure 2 for a description of what should be persistent.
func (rf *Raft) persist() {
	// Your code here.
	// Example:
	// w := new(bytes.Buffer)
	// e := gob.NewEncoder(w)
	// e.Encode(rf.xxx)
	// e.Encode(rf.yyy)
	// data := w.Bytes()
	// rf.persister.SaveRaftState(data)

	w := new(bytes.Buffer)
	e := gob.NewEncoder(w)

	e.Encode(rf.currentTerm)
	e.Encode(rf.votedFor)
	e.Encode(rf.log)

	data := w.Bytes()
	rf.persister.SaveRaftState(data)

}

// restore previously persisted state.
func (rf *Raft) readPersist(data []byte) {
	// Your code here.
	// Example:
	// r := bytes.NewBuffer(data)
	// d := gob.NewDecoder(r)
	// d.Decode(&rf.xxx)
	// d.Decode(&rf.yyy)

	r := bytes.NewBuffer(data)
	d := gob.NewDecoder(r)

	d.Decode(&rf.currentTerm)
	d.Decode(&rf.votedFor)
	d.Decode(&rf.log)

}

// example RequestVote RPC arguments structure.
type RequestVoteArgs struct {
	// Your data here.
	Term         int
	CandidateId  int
	LastLogIndex int
	LastLogTerm  int
}

// example RequestVote RPC reply structure.
type RequestVoteReply struct {
	// Your data here.
	Term        int
	VoteGranted bool
}

func (rf *Raft) getRequestVoteArgs() RequestVoteArgs {
	rf.mu.Lock()
	defer rf.mu.Unlock()

	lastLogIndex, lastLogTerm := rf.getLastLogIndexAndTerm()
	args := RequestVoteArgs{
		Term:         rf.currentTerm,
		CandidateId:  rf.me,
		LastLogIndex: lastLogIndex,
		LastLogTerm:  lastLogTerm,
	}
	return args
}

func (rf *Raft) getLastLogIndexAndTerm() (lastLogIndex int, lastLogTerm int) {
	last := len(rf.log) - 1
	lastLogIndex = rf.log[last].Index
	lastLogTerm = rf.log[last].Term

	assert(last, lastLogIndex, fmt.Sprintf("Server[%v](%s) %+v, The slice index should be equal to lastLogIndex", rf.me, rf.getRole(), rf.log))

	return lastLogIndex, lastLogTerm
}

// example RequestVote RPC handler.
func (rf *Raft) RequestVote(args RequestVoteArgs, reply *RequestVoteReply) {
	// Your code here.
	// TODO: Implement it so that servers will vote for one another
	rf.mu.Lock()
	DPrintf("[DEBUG] Svr[%v]:(%s, Term:%v) Start Func RequestVote with args:%+v", rf.me, rf.getRole(), rf.currentTerm, args)
	defer rf.mu.Unlock()
	defer DPrintf("[DEBUG] Svr[%v]:(%s) End Func RequestVote with args:%+v, reply:%+v", rf.me, rf.getRole(), args, reply)

	// 初始化
	reply.VoteGranted = false
	reply.Term = rf.currentTerm

	if rf.currentTerm > args.Term {
		return
	} else if rf.currentTerm < args.Term {
		rf.changeToFollower(args.Term)
	}

	// 是否可以进行投票
	if rf.rejectVote(args) {
		return
	}

	if rf.votedFor == args.CandidateId {
		reply.VoteGranted = true
		rf.resetElectionTimer()
		return
	} else {
		rf.currentTerm = args.Term
		rf.votedFor = args.CandidateId
		reply.VoteGranted = true
		rf.resetElectionTimer()
	}

	DPrintf("[DEBUG] Svr[%v]:(%s) Vote for %v", rf.me, rf.getRole(), args.CandidateId)
}

func (rf *Raft) rejectVote(args RequestVoteArgs) bool {
	if rf.role == LEADER {
		return true
	}
	if rf.votedFor != -1 && rf.votedFor != args.CandidateId {
		return true
	}
	// $5.4.1的限制
	lastLogIndex, lastLogTerm := rf.getLastLogIndexAndTerm()
	if lastLogTerm != args.LastLogTerm {
		return lastLogTerm > args.LastLogTerm
	}
	return lastLogIndex > args.LastLogIndex
}

// example code to send a RequestVote RPC to a server.
// server is the index of the target server in rf.peers[].
// expects RPC arguments in args.
// fills in *reply with RPC reply, so caller should
// pass &reply.
// the types of the args and reply passed to Call() must be
// the same as the types of the arguments declared in the
// handler function (including whether they are pointers).
//
// returns true if labrpc says the RPC was delivered.
//
// if you're having trouble getting RPC to work, check that you've
// capitalized all field names in structs passed over RPC, and
// that the caller passes the address of the reply struct with &, not
// the struct itself.
func (rf *Raft) sendRequestVote(server int, args RequestVoteArgs, reply *RequestVoteReply) bool {
	ok := rf.peers[server].Call("Raft.RequestVote", args, reply)
	return ok
}

func (rf *Raft) getElectionFromPeers() {
	DPrintf("[DEBUG] Svr[%v]:(%s) Begin sendRequestVoteRPCToOthers", rf.me, rf.getRole())
	n := len(rf.peers)
	voteCh := make(chan bool, n) // 接收来自各个节点的reply

	for server := range rf.peers {
		// 不发送给自己
		if server == rf.me {
			continue
		} else {
			args := rf.getRequestVoteArgs()
			// 开启新go routine，分别发送RequestVote给对应的server
			go func(server int) {
				reply := RequestVoteReply{}
				ok := rf.sendRequestVote(server, args, &reply)
				if ok {
					voteCh <- reply.VoteGranted
				} else {
					voteCh <- false // 如果没有收到回复，视为不投票
				}

			}(server)
		}
	}

	// 统计投票结果
	replyCounter := 1 // 收到的回复
	validCounter := 1 // 投自己的回复
	for {
		vote := <-voteCh
		rf.mu.Lock()
		if rf.role == FOLLOWER {
			rf.mu.Unlock()
			DPrintf("[DEBUG] Svr[%v]:(%s) has been a follower", rf.me, rf.getRole())
			return
		}
		rf.mu.Unlock()
		replyCounter++
		if vote == true {
			validCounter++
		}
		if replyCounter == n || // 所有人都投票了
			validCounter > n/2 || // 已经得到了majority投票
			replyCounter-validCounter > n/2 { // 已经有majority的投票人不是投自己
			break
		}
	}

	if validCounter > n/2 {
		// 得到了majority投票，成为Leader
		rf.mu.Lock()
		rf.changeToLeader()
		rf.mu.Unlock()
	} else {
		DPrintf("[DEBUG] Svr[%v]:(%s) get %v vote, Fails", rf.me, rf.getRole(), validCounter)
	}
}

type AppendEntriesArgs struct {
	Term         int
	LeaderId     int
	PrevLogIndex int
	PrevLogTerm  int
	Entries      []LogEntry
	LeaderCommit int
}

type AppendEntriesReply struct {
	Term      int
	Success   bool
	NextIndex int
}

func getMajoritySameIndex(matchIndex []int) int {
	num := len(matchIndex)
	tmpMatchIndex := make([]int, num)
	copy(tmpMatchIndex, matchIndex)
	sort.Sort(sort.Reverse(sort.IntSlice(tmpMatchIndex)))
	return tmpMatchIndex[num/2]
}

func (rf *Raft) getAppendLogs(slave int) (prevLogIndex int, prevLogTerm int, entries []LogEntry) {
	nextIndex := rf.nextIndex[slave]
	lastLogIndex, lastLogTerm := rf.getLastLogIndexAndTerm()
	if nextIndex <= 0 || nextIndex > lastLogIndex {
		prevLogIndex = lastLogIndex
		prevLogTerm = lastLogTerm
		return
	}

	entries = append([]LogEntry{}, rf.log[nextIndex:]...)
	prevLogIndex = nextIndex - 1
	if prevLogIndex == 0 {
		prevLogTerm = 0
	} else {
		prevLogTerm = rf.log[prevLogIndex].Term
	}
	return
}

func (rf *Raft) getAppendEntriesArgs(slave int) AppendEntriesArgs {
	prevLogIndex, prevLogTerm, entries := rf.getAppendLogs(slave)
	args := AppendEntriesArgs{
		Term:         rf.commitIndex,
		LeaderId:     rf.me,
		PrevLogIndex: prevLogIndex,
		PrevLogTerm:  prevLogTerm,
		Entries:      entries,
		LeaderCommit: rf.commitIndex,
	}
	return args
}

func (rf *Raft) getNextIndex() int {
	lastLogIndex, _ := rf.getLastLogIndexAndTerm()
	nextIndex := lastLogIndex + 1
	return nextIndex
}

//func (rf *Raft) AppendEntries(args AppendEntriesArgs, reply *AppendEntriesReply) {
//	rf.mu.Lock()
//	defer rf.persist()
//	defer rf.mu.Unlock()
//
//	reply.Success = false
//	reply.Term = rf.currentTerm
//
//	if rf.currentTerm > args.Term {
//		DPrintf("[DEBUG] Svr[%v]:(%s) Reject AppendEntries due to currentTerm > args.Term", rf.me, rf.getRole())
//		return
//	}
//
//	if len(args.Entries) == 0 {
//		DPrintf("[DEBUG] Svr[%v]:(%s, Term:%v) Get Heart Beats from %v", rf.me, rf.getRole(), rf.currentTerm, args.LeaderId)
//	} else {
//		DPrintf("[DEBUG] Svr[%v]:(%s, Term:%v) Start Func AppendEntries with args:%+v", rf.me, rf.getRole(), rf.currentTerm, args)
//		defer DPrintf("[DEBUG] Svr[%v]:(%s) End Func AppendEntries with args:%+v, reply:%+v", rf.me, rf.getRole(), args, reply)
//	}
//
//	rf.currentTerm = args.Term
//	rf.changeToFollower(args.Term)
//	rf.resetElectionTimer()
//
//	lastLogIndex, _ := rf.getLastLogIndexAndTerm()
//	if args.PrevLogIndex > lastLogIndex {
//		DPrintf("[DEBUG] Svr[%v]:(%s) Reject AppendEntries due to lastLogIndex < args.PrevLogIndex", rf.me, rf.getRole())
//		reply.NextIndex = rf.getNextIndex()
//	} else if rf.log[args.PrevLogIndex].Term != args.PrevLogTerm {
//		DPrintf("[DEBUG] Svr[%v]:(%s) Previous log entries do not match", rf.me, rf.getRole())
//		reply.NextIndex = BackOff
//	} else {
//		reply.Success = true
//		rf.log = append(rf.log[0:args.PrevLogIndex+1], args.Entries...)
//	}
//
//	if reply.Success {
//		rf.leaderID = args.LeaderId
//		if args.LeaderCommit > rf.commitIndex {
//			lastLogIndex, _ = rf.getLastLogIndexAndTerm()
//			rf.commitIndex = min(args.LeaderCommit, lastLogIndex)
//			DPrintf("[DEBUG] Svr[%v]:(%s) Follower Update commitIndex, lastLogIndex is %v", rf.me, rf.getRole(), lastLogIndex)
//
//		}
//	}
//}

// AppendEntries RPC handler.
func (rf *Raft) AppendEntries(args AppendEntriesArgs, reply *AppendEntriesReply) {
	// TODO:
	rf.mu.Lock()
	defer rf.persist()
	defer rf.mu.Unlock()

	// 初始化
	reply.Success = false
	reply.Term = rf.currentTerm

	// 拒绝Term小于自己的节点的Append请求
	if rf.currentTerm > args.Term {
		// reply false if term < currentTerm
		DPrintf("[DEBUG] Svr[%v]:(%s) Reject AppendEntries due to currentTerm > args.Term", rf.me, rf.getRole())
		return
	}

	// 判断是否是来自leader的心跳
	if len(args.Entries) == 0 {
		DPrintf("[DEBUG] Svr[%v]:(%s, Term:%v) Get Heart Beats from %v", rf.me, rf.getRole(), rf.currentTerm, args.LeaderId)
	} else {
		DPrintf("[DEBUG] Svr[%v]:(%s, Term:%v) Start Func AppendEntries with args:%+v", rf.me, rf.getRole(), rf.currentTerm, args)
		defer DPrintf("[DEBUG] Svr[%v]:(%s) End Func AppendEntries with args:%+v, reply:%+v", rf.me, rf.getRole(), args, reply)
	}

	rf.currentTerm = args.Term
	rf.changeToFollower(args.Term)
	rf.resetElectionTimer() // 收到了有效的Leader的消息，重置选举的定时器

	// 考虑rf.log[args.PrevLogIndex]有没有内容，即上一个应该同步的位置
	lastLogIndex, _ := rf.getLastLogIndexAndTerm()
	if args.PrevLogIndex > lastLogIndex {
		DPrintf("[DEBUG] Svr[%v]:(%s) Reject AppendEntries due to lastLogIndex < args.PrevLogIndex", rf.me, rf.getRole())
		reply.NextIndex = rf.getNextIndex()
	} else if rf.log[args.PrevLogIndex].Term != args.PrevLogTerm {
		DPrintf("[DEBUG] Svr[%v]:(%s) Previous log entries do not match", rf.me, rf.getRole())
		reply.NextIndex = BackOff
	} else {
		reply.Success = true
		rf.log = append(rf.log[0:args.PrevLogIndex+1], args.Entries...) // [a:b]，左取右不取，如果有冲突就直接截断
	}

	if reply.Success {
		rf.leaderID = args.LeaderId
		if args.LeaderCommit > rf.commitIndex {
			lastLogIndex, _ := rf.getLastLogIndexAndTerm()
			rf.commitIndex = min(args.LeaderCommit, lastLogIndex)
			DPrintf("[DEBUG] Svr[%v]:(%s) Follower Update commitIndex, lastLogIndex is %v", rf.me, rf.getRole(), lastLogIndex)
		}
	}

}

func (rf *Raft) sendAppendEntries(server int, args AppendEntriesArgs, reply *AppendEntriesReply) bool {
	ok := rf.peers[server].Call("Raft.AppendEntries", args, reply)
	return ok
}

func (rf *Raft) sendAppendEntriesToPeer(slave int) {
	rf.mu.Lock()
	if rf.role != LEADER {
		rf.mu.Unlock()
		return
	}
	args := rf.getAppendEntriesArgs(slave)

	if len(args.Entries) > 0 {
		DPrintf("[DEBUG] Svr[%v]:(%s) sendAppendEntriesRPCToPeer send to Svr[%v]", rf.me, rf.getRole(), slave)
	}

	rf.mu.Unlock()
	reply := AppendEntriesReply{}
	ok := rf.sendAppendEntries(slave, args, &reply)
	if ok {
		rf.mu.Lock()
		if reply.Term > rf.currentTerm {
			DPrintf("[DEBUG] Svr[%v] (%s) Get reply for AppendEntries from %v, reply.Term > rf.currentTerm", rf.me, rf.getRole(), slave)
			rf.changeToFollower(reply.Term)
			rf.resetElectionTimer()
			rf.mu.Unlock()
			return
		}

		if rf.role != LEADER || rf.currentTerm != args.Term {
			rf.mu.Unlock()
			return
		}

		DPrintf("[DEBUG] Svr[%v] (%s) Get reply for AppendEntries from %v, reply.Term <= rf.currentTerm, reply is %+v", rf.me, rf.getRole(), slave, reply)
		if reply.Success {
			lenEntries := len(args.Entries)
			rf.matchIndex[slave] = args.PrevLogIndex + lenEntries
			rf.nextIndex[slave] = rf.matchIndex[slave] + 1
			DPrintf("[DEBUG] Svr[%v] (%s): matchIndex[%v] is %v", rf.me, rf.getRole(), slave, rf.matchIndex[slave])
			majorityIndex := getMajoritySameIndex(rf.matchIndex)

			if rf.log[majorityIndex].Term == rf.currentTerm && majorityIndex > rf.commitIndex {
				rf.commitIndex = majorityIndex
				DPrintf("[DEBUG] Svr[%v] (%s): Update commitIndex to %v", rf.me, rf.getRole(), rf.commitIndex)
			}
		} else {
			DPrintf("[DEBUG] Svr[%v] (%s): append to Svr[%v]Success is False, reply is %+v", rf.me, rf.getRole(), slave, &reply)
			if reply.NextIndex > 0 {
				rf.nextIndex[slave] = reply.NextIndex

			} else if reply.NextIndex == BackOff {
				prevIndex := args.PrevLogIndex
				for prevIndex > 0 && rf.log[prevIndex].Term == args.PrevLogTerm {
					prevIndex -= 1
				}
				rf.nextIndex[slave] = prevIndex + 1
			}
		}
		rf.mu.Unlock()
	}
}

// the service using Raft (e.g. a k/v server) wants to start
// agreement on the next command to be appended to Raft's log. if this
// server isn't the leader, returns false. otherwise start the
// agreement and return immediately. there is no guarantee that this
// command will ever be committed to the Raft log, since the leader
// may fail or lose an election.
//
// the first return value is the index that the command will appear at
// if it's ever committed. the second return value is the current
// Term. the third return value is true if this server believes it is
// the leader.
func (rf *Raft) Start(command interface{}) (int, int, bool) {
	index := -1
	term := -1
	isLeader := true

	rf.mu.Lock()
	lastLogIndex, _ := rf.getLastLogIndexAndTerm()
	index = lastLogIndex + 1
	term = rf.currentTerm
	isLeader = rf.role == LEADER

	if isLeader {
		logEntry := LogEntry{
			Command: command,
			Term:    term,
			Index:   index,
		}
		rf.log = append(rf.log, logEntry)
		rf.matchIndex[rf.me] = index
		rf.persist()
	}

	rf.mu.Unlock()
	return index, term, isLeader
}

// the tester calls Kill() when a Raft instance won't
// be needed again. you are not required to do anything
// in Kill(), but it might be convenient to (for example)
// turn off debug output from this instance.
func (rf *Raft) Kill() {
	// Your code here, if desired.

}

func (rf *Raft) resetElectionTimer() {
	randInt := rand.Intn(ElectionTimeoutMax-ElectionTimeoutMin) + ElectionTimeoutMin
	timeout := time.Duration(randInt) * time.Millisecond
	rf.electionTimer.Reset(timeout)
}

func (rf *Raft) resetHeartBeatTimer() {
	timeout := HeartBeatInterval * time.Millisecond
	rf.heartbeatTimer.Reset(timeout)
}

func (rf *Raft) changeToCandidate() {
	rf.currentTerm += 1
	rf.role = CANDIDATE
	rf.votedFor = rf.me
	rf.resetElectionTimer()
	rf.persist()
	DPrintf("[DEBUG] Svr[%v]:(%s) switch to candidate.", rf.me, rf.getRole())

}

func (rf *Raft) changeToFollower(term int) {
	var flag bool
	if rf.role != FOLLOWER {
		flag = true
	}

	rf.role = FOLLOWER
	rf.currentTerm = term
	rf.votedFor = -1

	if flag {
		DPrintf("[DEBUG] Svr[%v]:(%s) switch to follower.", rf.me, rf.getRole())
	}

	rf.persist()

}

func (rf *Raft) changeToLeader() {
	rf.role = LEADER
	rf.leaderID = rf.me
	rf.resetElectionTimer()
	rf.heartbeatTimer.Reset(0)

	numServer := len(rf.peers)
	numLog := len(rf.log)
	rf.matchIndex = make([]int, numServer)
	rf.nextIndex = make([]int, numServer)

	for i := range rf.peers {
		rf.matchIndex[i] = 0
		rf.nextIndex[i] = numLog
	}

	rf.matchIndex[rf.me] = numLog - 1
	rf.persist()
	DPrintf("[DEBUG] Svr[%v]:(%s) switch to leader and reset heartBeatTimer 0.", rf.me, rf.getRole())

}

func (rf *Raft) ElectLeader() {
	for {
		// 等待 election timeout
		<-rf.electionTimer.C // 表达式会被Block直到超时
		rf.resetElectionTimer()

		rf.mu.Lock()
		DPrintf("[DEBUG] Svr[%v]:(%s) Start Leader Election Loop", rf.me, rf.getRole())
		if rf.role == LEADER {
			DPrintf("[DEBUG] Svr[%v]:(%s) End Leader Election Loop, Leader is Svr[%v]", rf.me, rf.getRole(), rf.me)
			rf.mu.Unlock()
			continue
		}

		if rf.role == FOLLOWER || rf.role == CANDIDATE {
			rf.changeToCandidate()
			rf.mu.Unlock()
			rf.getElectionFromPeers()
		}
	}
}

func (rf *Raft) HeartBeat() {
	for {
		<-rf.heartbeatTimer.C
		rf.resetHeartBeatTimer()

		rf.mu.Lock()
		if rf.role != LEADER {
			rf.mu.Unlock()
			continue
		}
		rf.mu.Unlock()

		for slave := range rf.peers {
			if slave == rf.me {
				rf.nextIndex[slave] = len(rf.log) + 1
				rf.matchIndex[slave] = len(rf.log)
				continue
			} else {
				go rf.sendAppendEntriesToPeer(slave)
			}
		}
	}
}

func (rf *Raft) applyMsgChan(index int) {
	msg := ApplyMsg{
		Index:       index,
		Command:     rf.log[index].Command,
		UseSnapshot: false,
		Snapshot:    nil,
	}
	DPrintf("[DEBUG] Srv[%v](%s) apply log entry %+v", rf.me, rf.getRole(), rf.log[index].Command)
	rf.applyChan <- msg
}

func (rf *Raft) Apply() {
	for {
		time.Sleep(10 * time.Millisecond)
		rf.mu.Lock()
		for rf.lastApplied < rf.commitIndex {
			rf.lastApplied += 1
			rf.applyMsgChan(rf.lastApplied)
		}
		rf.mu.Unlock()
	}
}

// the service or tester wants to create a Raft server. the ports
// of all the Raft servers (including this one) are in peers[]. this
// server's port is peers[me]. all the servers' peers[] arrays
// have the same order. persister is a place for this server to
// save its persistent state, and also initially holds the most
// recent saved state, if any. applyCh is a channel on which the
// tester or service expects Raft to send ApplyMsg messages.
// Make() must return quickly, so it should start goroutines
// for any long-running work.
func Make(peers []*labrpc.ClientEnd, me int,
	persister *Persister, applyCh chan ApplyMsg) *Raft {

	DPrintf("[DEBUG] Svr[%v]: Start Func Make()\n", me)
	defer DPrintf("[DEBUG] Svr[%v]: End Func Make()\n", me)

	rf := &Raft{}
	rf.peers = peers
	rf.persister = persister
	rf.me = me

	// Your initialization code here.
	rf.currentTerm = 0
	rf.votedFor = -1
	rf.lastApplied = 0
	rf.commitIndex = 0
	rf.applyChan = applyCh
	rf.role = FOLLOWER
	rf.leaderID = -1

	guideEntry := LogEntry{
		Command: nil,
		Term:    0,
		Index:   0,
	}
	rf.log = append(rf.log, guideEntry)

	// initialize from state persisted before a crash
	rf.readPersist(persister.ReadRaftState())

	rf.electionTimer = time.NewTimer(100 * time.Millisecond)
	rf.heartbeatTimer = time.NewTimer(HeartBeatInterval * time.Millisecond)

	go rf.ElectLeader()
	go rf.HeartBeat()
	go rf.Apply()

	return rf
}

func (rf *Raft) logToString() string {
	res := ""
	for index, log := range rf.log {
		if index <= rf.commitIndex {
			res += fmt.Sprintf("{C*%v T*%v i*%v}", log.Command, log.Term, log.Index)
		} else {
			res += fmt.Sprintf("{C:%v T:%v i:%v}", log.Command, log.Term, log.Index)

		}
	}
	return res
}

func (rf *Raft) toString() string {
	return fmt.Sprintf("LID: %v;Term:%d;log:%s;commitIndex:%v;",
		rf.leaderID, rf.currentTerm, rf.logToString(), rf.commitIndex)
}

func (rf *Raft) toStringWithoutLog() string {
	return fmt.Sprintf("LID: %v;Term:%d;commitIndex:%v;",
		rf.leaderID, rf.currentTerm, rf.commitIndex)
}

func (rf *Raft) getRole() string {
	var role string
	switch rf.role {
	case LEADER:
		role = "Lead"
	case FOLLOWER:
		role = "Foll"
	case CANDIDATE:
		role = "Cand"
	}
	//return role + " " + rf.toStringWithoutLog()
	return role + " " + rf.toString()
	//return role
}

func assert(a interface{}, b interface{}, msg interface{}) {
	if a != b {
		panic(msg)
	}
}
