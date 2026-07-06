package util

import (
	"fmt"
	"sync"
	"testing"
)

type Class struct {
	name string
	Id   int
}

func (n *Class) GetKey() string {
	return n.name
}

func TestCollection(t *testing.T) {
	var cc Collection[string, *Class]
	for i := 0; i < 10; i++ {
		cc.Add(&Class{name: fmt.Sprintf("%d", i), Id: i})
	}
	cc.RemoveByKey("1")
	if item, ok := cc.Get("1"); ok {
		fmt.Println(item)
	} else {
		fmt.Println("not found", item)
	}
}

func TestCollection_Range(t *testing.T) {
	var cc Collection[string, *Class]
	for i := 0; i < 10; i++ {
		cc.Add(&Class{name: fmt.Sprintf("%d", i), Id: i})
	}
	for item := range cc.Range {
		fmt.Println(item)
		cc.Remove(item)
	}
}

// TestItem 是用于测试的结构体
type TestItem struct {
	ID   string
	Data string
}

func (t TestItem) GetKey() string {
	return t.ID
}

func TestCollection_BasicOperations(t *testing.T) {
	c := &Collection[string, TestItem]{
		L: &sync.RWMutex{},
	}

	// 测试 Add
	item1 := TestItem{ID: "1", Data: "test1"}
	c.Add(item1)
	if c.Length != 1 {
		t.Errorf("Expected length 1, got %d", c.Length)
	}

	// 测试 Get
	got, ok := c.Get("1")
	if !ok || got != item1 {
		t.Errorf("Expected to get item1, got %v", got)
	}

	// 测试 AddUnique
	ok = c.AddUnique(item1)
	if ok || c.Length != 1 {
		t.Error("AddUnique should not add duplicate item")
	}

	// 测试 Set
	item1Modified := TestItem{ID: "1", Data: "test1-modified"}
	added := c.Set(item1Modified)
	if added {
		t.Error("Set should not return true for existing item")
	}
	got, _ = c.Get("1")
	if got.Data != "test1-modified" {
		t.Errorf("Expected modified data, got %s", got.Data)
	}

	// 测试 Remove
	if !c.Remove(item1) {
		t.Error("Remove should return true for existing item")
	}
	if c.Length != 0 {
		t.Errorf("Expected length 0 after remove, got %d", c.Length)
	}
}

func TestCollection_Events(t *testing.T) {
	c := &Collection[string, TestItem]{}

	var addCalled, removeCalled bool
	c.OnAdd(func(item TestItem) {
		addCalled = true
		if item.ID != "1" {
			t.Errorf("Expected item ID 1, got %s", item.ID)
		}
	})

	c.OnRemove(func(item TestItem) {
		removeCalled = true
		if item.ID != "1" {
			t.Errorf("Expected item ID 1, got %s", item.ID)
		}
	})

	item := TestItem{ID: "1", Data: "test"}
	c.Add(item)
	if !addCalled {
		t.Error("Add listener was not called")
	}

	c.Remove(item)
	if !removeCalled {
		t.Error("Remove listener was not called")
	}
}

func TestCollection_Search(t *testing.T) {
	c := &Collection[string, TestItem]{}
	items := []TestItem{
		{ID: "1", Data: "test1"},
		{ID: "2", Data: "test2"},
		{ID: "3", Data: "test1"},
	}

	for _, item := range items {
		c.Add(item)
	}

	// 测试 Find
	found, ok := c.Find(func(item TestItem) bool {
		return item.Data == "test1"
	})
	if !ok || found.ID != "1" {
		t.Error("Find should return first matching item")
	}

	// 测试 Search
	count := 0
	search := c.Search(func(item TestItem) bool {
		return item.Data == "test1"
	})
	search(func(item TestItem) bool {
		count++
		return true
	})
	if count != 2 {
		t.Errorf("Search should find 2 items, found %d", count)
	}
}

func TestCollection_ConcurrentAccess(t *testing.T) {
	c := &Collection[string, TestItem]{
		L: &sync.RWMutex{},
	}

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			item := TestItem{ID: string(rune(id)), Data: "test"}
			c.Add(item)
		}(i)
	}
	wg.Wait()

	if c.Length != 100 {
		t.Errorf("Expected 100 items, got %d", c.Length)
	}
}

func TestCollection_SetConcurrentWithClear(t *testing.T) {
	c := &Collection[string, TestItem]{L: &sync.RWMutex{}}
	for i := 0; i < 10; i++ {
		c.Set(TestItem{ID: fmt.Sprintf("%d", i), Data: "v"})
	}
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func(n int) {
			defer wg.Done()
			c.Set(TestItem{ID: fmt.Sprintf("%d", n%10), Data: "updated"})
		}(i)
		go func() {
			defer wg.Done()
			c.Clear()
		}()
	}
	wg.Wait()
}
