package deque

type Deque[T any] struct {
	entries []T
	head    int
	tail    int
	count   int
}

func NewDeque[T any](initialCap ...int) *Deque[T] {
	cap := 128
	if len(initialCap) > 0 && initialCap[0] > 0 {
		cap = initialCap[0]
	}
	return &Deque[T]{
		entries: make([]T, cap),
		head:    0,
		tail:    0,
		count:   0,
	}
}

func (d *Deque[T]) Len() int {
	return d.count
}

func (d *Deque[T]) grow() {
	newEntries := make([]T, len(d.entries)*2)
	for i := 0; i < d.count; i++ {
		newEntries[i] = d.entries[(d.head+i)%len(d.entries)]
	}
	d.entries = newEntries
	d.head = 0
	d.tail = d.count
}

func (d *Deque[T]) PushBack(x ...T) {
	for _, x := range x {
		if d.count == len(d.entries) {
			d.grow()
		}
		d.entries[d.tail] = x
		d.count++
		d.tail = (d.tail + 1) % len(d.entries)
	}
}

func (d *Deque[T]) PushFront(x T) {
	if d.count == len(d.entries) {
		d.grow()
	}
	d.head = (d.head - 1 + len(d.entries)) % len(d.entries)
	d.entries[d.head] = x
	d.count++
}

func (d *Deque[T]) PopBack() (T, bool) {
	if d.count == 0 {
		var z T
		return z, false
	}
	d.tail = (d.tail - 1 + len(d.entries)) % len(d.entries)
	x := d.entries[d.tail]
	var zero T
	d.entries[d.tail] = zero
	d.count--
	return x, true
}

func (d *Deque[T]) PopFront() (T, bool) {
	if d.count == 0 {
		var z T
		return z, false
	}
	x := d.entries[d.head]
	var zero T
	d.entries[d.head] = zero
	d.head = (d.head + 1) % len(d.entries)
	d.count--
	return x, true
}

func (d *Deque[T]) PeekFront() (T, bool) {
	if d.count == 0 {
		var z T
		return z, false
	}
	return d.entries[d.head], true
}

func (d *Deque[T]) PeekBack() (T, bool) {
	if d.count == 0 {
		var z T
		return z, false
	}
	return d.entries[(d.tail-1+len(d.entries))%len(d.entries)], true
}
