package reudp

import (
    "testing"
    "time"
    "net"
    "log"
)

var in,out *Reudp

var testDataSmall = []byte("Jackdaws loves my big sphinx of quartz")
var testDataBig = []byte("Lorem ipsum and all the other wise things that matter. Lorem ipsum and all the other wise things that matter. Lorem ipsum and all the other wise things that matter. Lorem ipsum and all the other wise things that matter. Lorem ipsum and all the other wise things that matter. Lorem ipsum and all the other wise things that matter.")
var testDataHuge = []byte("Introduction. The Go memory model specifies the conditions under which reads of a variable in one goroutine can be guaranteed to observe values produced by writes to the same variable in a different goroutine. Advice. Programs that modify data being simultaneously accessed by multiple goroutines must serialize such access. To serialize access, protect the data with channel operations or other synchronization primitives such as those in the sync and sync/atomic packages. If you must read the rest of this document to understand the behavior of your program, you are being too clever. Don't be clever. Happens Before. Within a single goroutine, reads and writes must behave as if they executed in the order specified by the program. That is, compilers and processors may reorder the reads and writes executed within a single goroutine only when the reordering does not change the behavior within that goroutine as defined by the language specification. Because of this reordering, the execution order observed by one goroutine may differ from the order perceived by another. For example, if one goroutine executes a = 1; b = 2;, another might observe the updated value of b before the updated value of a. To specify the requirements of reads and writes, we define happens before, a partial order on the execution of memory operations in a Go program. If event e1 happens before event e2, then we say that e2 happens after e1. Also, if e1 does not happen before e2 and does not happen after e2, then we say that e1 and e2 happen concurrently. Within a single goroutine, the happens-before order is the order expressed by the program. A read r of a variable v is allowed to observe a write w to v if both of the following hold: r does not happen before w. There is no other write w' to v that happens after w but before r. To guarantee that a read r of a variable v observes a particular write w to v, ensure that w is the only write r is allowed to observe. That is, r is guaranteed to observe w if both of the following hold: w happens before r. Any other write to the shared variable v either happens before w or after r. This pair of conditions is stronger than the first pair; it requires that there are no other writes happening concurrently with w or r. Within a single goroutine, there is no concurrency, so the two definitions are equivalent: a read r observes the value written by the most recent write w to v. When multiple goroutines access a shared variable v, they must use synchronization events to establish happens-before conditions that ensure reads observe the desired writes. The initialization of variable v with the zero value for v's type behaves as a write in the memory model. Reads and writes of values larger than a single machine word behave as multiple machine-word-sized operations in an unspecified order.")

func init() {
	in,out = New(data,err,logg),New(data,err,logg)
	
	inaddr,_ := net.ResolveUDPAddr("udp", "localhost:0")
	outaddr,_ := net.ResolveUDPAddr("udp", "localhost:0")
	
	if err := in.Listen(inaddr); err != nil {
		log.Fatalln(err)
	}
	if err := out.Listen(outaddr); err != nil {
		log.Fatalln(err)
	}
}

func data(who *net.UDPAddr, data []byte) {
	log.Printf(" ------  from %v : data %v", who, string(data))
}

func err(who *net.UDPAddr, err error) {
	log.Printf(" ------  err from %v : %v", who, err)
}

func logg(format string, args ...interface{}) {
	//log.Printf(format, args...)
}

func TestLittleDataInOut(t *testing.T) {
	err := in.Send(out.LocalAddr(), testDataSmall)
	
	if err != nil {
		t.Error(err)
	}
	
	<-time.Tick(time.Second * 2)
	
	err = out.Send(in.LocalAddr(), testDataSmall)
	
	if err != nil {
		t.Error(err)
	}
}

func TestBigDataInOut(t *testing.T) {
	err := in.Send(out.LocalAddr(), testDataBig)
	
	if err != nil {
		t.Error(err)
	}
	
	<-time.Tick(time.Second * 2)
	
	err = out.Send(in.LocalAddr(), testDataBig)
	
	if err != nil {
		t.Error(err)
	}
}

func TestHugeDataInOut(t *testing.T) {
	err := in.Send(out.LocalAddr(), testDataHuge)
	
	if err != nil {
		t.Error(err)
	}
	
	<-time.Tick(time.Second * 2)
	
	err = out.Send(in.LocalAddr(), testDataHuge)
	
	if err != nil {
		t.Error(err)
	}
}

