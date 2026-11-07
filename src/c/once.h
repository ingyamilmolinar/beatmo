/* The prewarm pool's first renders of a session are concurrent, and the table generators memset then SUM harmonics: a
 * plain ready flag let two cold renders build at once and froze a corrupted table for the whole process. beatmo_once
 * runs fn exactly once; every other caller waits. Plain loads and stores in the single-threaded browser. */
#ifndef BEATMO_ONCE_H
#define BEATMO_ONCE_H

typedef struct {
    int state; /* 0 = not run, 1 = running, 2 = done */
} beatmo_once_t;

#define BEATMO_ONCE_INIT {0}

static inline void beatmo_once(beatmo_once_t *o, void (*fn)(void)) {
    if (__atomic_load_n(&o->state, __ATOMIC_ACQUIRE) == 2) {
        return;
    }
    int expected = 0;
    if (__atomic_compare_exchange_n(&o->state, &expected, 1, 0,
                                    __ATOMIC_ACQ_REL, __ATOMIC_ACQUIRE)) {
        fn();
        __atomic_store_n(&o->state, 2, __ATOMIC_RELEASE);
        return;
    }
    /* The build takes a few milliseconds, once per process. */
    while (__atomic_load_n(&o->state, __ATOMIC_ACQUIRE) != 2) {
    }
}

#endif /* BEATMO_ONCE_H */
