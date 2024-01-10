<div align='center' ><font size='70'><b>实验报告</b></font></div>



# 1. 问题分析

Raft协议是分布式系统中用于实现多节点之间数据一致性的算法，Raft协议的系统中包含Leader节点和Follower节点，协议内容主要包括领导选举、日志复制、安全性三个部分。

本次实验要求实现基于Raft协议的分布式系统的核心逻辑部分，采用GO语言实现领导节点选举、日志追加复制等功能。

其中，第一部分要求实现领导节点选举的完整生命周期，这需要实现心跳信息的发送、身份的切换、投票信息的发送、节点的投票统计等内容，从而完成分布式系统中领导节点、跟随节点的身份问题；第二部分

# 2. 系统设计与实现

在本节中，将分别阐述本实验两个部分的实现思路和部分实现细节。

## Part 1: 领导选举

### 节点身份
在Raft协议中，节点共有三种可能的身份：`Leader`, `Follower`, `Candidate`，因此首先定义身份常量如下：
```
LEADER    = 1
FOLLOWER  = 2
CANDIDATE = 3
```

`LEADER`表示当前节点为领导节点，它会定期向所有节点广播心跳信息来确认自己的存在；`FOLLOWER`表示跟随节点，接收来自领导节点的信息；`CANDIDATE`表示候选节点，当未收到领导节点的心跳信息时，所有节点自动成为候选节点，重新选举领导节点。

### 定时心跳信息
Leader节点每一时刻在系统中唯一存在，它通过定期向其他Follower节点发送心跳信息（`heartbeat`）来确认他的存在，我们通过在节点中定义心跳定时器`heartbeatTimer`来实现定期的心跳信息发送，心跳计时器结构如下：
```
heartbeatTimer *time.Timer
```

心跳计时器设定固定的时间间隔`HeartBeatInterval`，计时器每次超时，若领导节点未崩溃，则会发送一次心跳信息并重置心跳计时器。

重置计时器逻辑如下：
```GO
func (rf *Raft) resetHeartBeatTimer() {
	timeout := HeartBeatInterval * time.Millisecond
	rf.heartbeatTimer.Reset(timeout)
}

```

通过发送`AppendEntries`告知所有节点心跳信息， 发送心跳信息逻辑如下：

```GO
//为了逻辑简洁，每个节点都会启动heartbeat计时器，
//但是在发送函数中会检查节点身份从而只有Leader节点可以发送
for peer := range rf.peers {
    if peer == rf.me {
        rf.nextIndex[peer] = len(rf.log) + 1
        rf.matchIndex[peer] = len(rf.log)
    } else {
	    //使用协程，并行发送信息 
	    go rf.sendAppendEntriesToPeer(peer)
	}
}
```

其中，`sendAppendEntriesToPeer`表示向其他节点发送`AppendEntries`信息，会调用`sendAppendEntries`来完成RPC信息的传递，`AppendEntries`的具体内容会在Part2中完成日志追加时具体叙述。

### 选举领导节点

当系统初始时或领导节点崩溃时，需要重新选举领导节点，领导节点的选举周期由选举时钟`electionTimer`控制，

```GO

for {
		<-rf.electionTimer.C
		rf.resetElectionTimer()

		rf.mu.Lock()
		if rf.role == LEADER {
			rf.mu.Unlock()
			continue
		} else if rf.role == FOLLOWER || rf.role == CANDIDATE {
			rf.changeToCandidate()
			rf.mu.Unlock()
			rf.getElectionFromPeers()

		}
	}

```

在实验中，我们通过`Make()`启动GO的协程（`GO routine`）来重复执行上述过程，从而维护Raft协议中的选举生命周期。

## Part 2: 日志追加
在Part1中我们可以知道Leader节点通过向其他节点发送心跳信息表示自己的存在，在实际实现中，Leader节点发送的`AppendEntries`也包括了日志追加部分内容，

# 3. 实验演示

## Part 1 实验结果
- Election

`TestInitialElection`、`TestReElection`测试结果如下图所示：

![](pics/election.png)

从图中可以看出，实现的Raft协议可以完成基本的选举以及在断网重连的情况下完成选举。

## Part 2 实验结果
- Agree

`TestBasicAgree`、`TestFailAgree`、`TestFailNoAgree`、`TestUnreliableAgree`测试结果如下图所示：

![](pics/agree.png)

从图中可以看出

- ConcurrentStarts

`TestConcurrentStarts`测试结果如下图所示：

![](pics/concurrentStart.png)

从图中可以看出

- Rejoin

`TestRejoin`测试结果如下图所示：

![](pics/rejoin.png)

从图中可以看出

- Backup

`TestBackup`测试结果

![](pics/backup.png)

从图中可以看出

- Count

`TestCount`测试结果

![](pics/count.png)

从图中可以看出

# 4. 总结

在本次实验中，我熟悉使用了并发性能较强的GO语言，并通过实现Raft协议的部分核心逻辑，深入了解了Raft协议的实现细节以及一些设计的意图，同时也进一步理解了在分布式系统中完成一致性的难度。

实验过程中，我主要碰到了以下问题：


