// native_stubs.c is used only by the linkprobe described in the parent
// directory's README. It lets wasm-ld finish the native half of the cgo
// program before the real Go/native ABI adapters exist.
#include <pthread.h>
#include <stdatomic.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>

enum {
	thread_count = 4,
	allocation_size = 256 * 1024,
};

static _Thread_local uint32_t tls_value;
static atomic_uint completed;

static void *probe_thread(void *opaque) {
	uint32_t id = (uint32_t)(uintptr_t)opaque;
	unsigned char *allocation;

	if (tls_value != 0)
		return (void *)(uintptr_t)1;
	tls_value = id;

	allocation = malloc(allocation_size);
	if (allocation == NULL)
		return (void *)(uintptr_t)2;
	memset(allocation, (int)id, allocation_size);
	if (allocation[0] != id || allocation[allocation_size - 1] != id)
		return (void *)(uintptr_t)3;
	free(allocation);

	if (tls_value != id)
		return (void *)(uintptr_t)4;
	atomic_fetch_add_explicit(&completed, 1, memory_order_relaxed);
	return NULL;
}

uintptr_t _cgo_topofstack(void) {
	return 0;
}

void crosscall1(void (*fn)(void), void *arg, int32_t argsize) {
	(void)fn;
	(void)arg;
	(void)argsize;
}

int __main_argc_argv_envp(int argc, char **argv, char **envp) {
	pthread_t threads[thread_count];
	unsigned i;

	(void)argc;
	(void)argv;
	(void)envp;

	for (i = 0; i < thread_count; i++) {
		if (pthread_create(&threads[i], NULL, probe_thread,
		    (void *)(uintptr_t)(i + 1)) != 0)
			return 10 + (int)i;
	}
	for (i = 0; i < thread_count; i++) {
		void *result = NULL;
		if (pthread_join(threads[i], &result) != 0)
			return 20 + (int)i;
		if (result != NULL)
			return 30 + (int)(uintptr_t)result;
	}
	if (atomic_load_explicit(&completed, memory_order_relaxed) != thread_count)
		return 40;
	return 0;
}
