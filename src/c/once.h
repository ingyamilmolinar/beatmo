/* once.h — a thread-safe one-time initialiser for the engine's lazily built
 * tables.
 *
 * The voice prewarm pool renders on several threads at once from the moment a
 * circuit loads, so the FIRST renders of a session are concurrent. A plain
 * `if (!ready) { build(); ready = 1; }` let two of them build the same table
 * at the same time: the generators memset a table and then SUM harmonics into
 * it, so one render zeroed what another was summing (or both summed — a
 * doubled table), and the flag then froze the corrupted table for the rest of
 * the process. Guard: TestColdConcurrentRendersMatchSerial.
 *
 * beatmo_once runs fn exactly once; every other caller waits until it has
 * finished. Compiler __atomic builtins (gcc / clang / emscripten); in the
 * single-threaded browser build they are plain loads and stores. */
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
    /* Another thread is building the tables: they take a few milliseconds,
     * once per process. */
    while (__atomic_load_n(&o->state, __ATOMIC_ACQUIRE) != 2) {
    }
}

#endif /* BEATMO_ONCE_H */
