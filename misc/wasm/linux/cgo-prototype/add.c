// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include <stdint.h>
#include <pthread.h>
#include <errno.h>
#include <stdlib.h>

typedef struct {
	uint32_t a;
	uint64_t b;
} prototype_pair;

typedef struct {
	uint32_t *data;
	uint32_t length;
} prototype_buffer;

typedef struct {
	uint32_t *items[2];
} prototype_pointer_table;

typedef uint32_t (*prototype_transform)(uint32_t);

extern uint32_t GoCallback(uint32_t);
extern uint32_t GoPointerCallback(uint32_t *);
extern uint32_t *GoPointerReturn(uint32_t *);
extern prototype_buffer GoBufferCallback(prototype_buffer);

uint32_t
prototype_add32(uint32_t a, uint32_t b)
{
	return a + b;
}

uint64_t
prototype_mix64(uint64_t a, uint64_t b)
{
	return a ^ b;
}

uint32_t
prototype_bump(uint32_t *value)
{
	return ++*value;
}

uint32_t
prototype_checksum(const uint8_t *data, uint32_t length)
{
	uint32_t result = 0;
	uint32_t i;
	for (i = 0; i < length; i++)
		result += data[i];
	return result;
}

uint64_t
prototype_pair_sum(prototype_pair pair)
{
	return pair.a + pair.b;
}

uint32_t
prototype_buffer_sum(prototype_buffer buffer)
{
	uint32_t result = 0;
	uint32_t i;
	for (i = 0; i < buffer.length; i++)
		result += buffer.data[i];
	return result;
}

prototype_buffer
prototype_make_buffer(uint32_t *data, uint32_t length)
{
	prototype_buffer result = { data, length };
	return result;
}

uint32_t
prototype_pointer_table_sum(prototype_pointer_table table)
{
	return *table.items[0] + *table.items[1];
}

uint32_t
prototype_increment(uint32_t value)
{
	return value + 1;
}

prototype_transform
prototype_get_transform(void)
{
	return prototype_increment;
}

uint32_t
prototype_apply(prototype_transform transform, uint32_t value)
{
	return transform(value);
}

uint32_t
prototype_set_errno(uint32_t value)
{
	errno = (int)value;
	return 7;
}

uint32_t
prototype_call_go(uint32_t value)
{
	return GoCallback(value) + 1;
}

uint32_t
prototype_call_go_pointer(uint32_t value)
{
	return GoPointerCallback(&value) + 1;
}

uint32_t
prototype_call_go_pointer_return(uint32_t value)
{
	uint32_t *input = malloc(sizeof(*input));
	uint32_t *result;
	uint32_t output;
	if (input == NULL)
		return UINT32_MAX;
	*input = value;
	result = GoPointerReturn(input);
	output = result == input ? *result + 1 : UINT32_MAX;
	free(input);
	return output;
}

uint32_t
prototype_call_go_buffer(uint32_t *data, uint32_t length)
{
	uint32_t *copy = malloc(length * sizeof(*copy));
	prototype_buffer result;
	uint32_t output;
	uint32_t i;
	if (copy == NULL)
		return UINT32_MAX;
	for (i = 0; i < length; i++)
		copy[i] = data[i];
	result = GoBufferCallback((prototype_buffer){ copy, length });
	output = prototype_buffer_sum(result);
	free(copy);
	return output;
}

// Compiler TLS: each thread starts from the initialized image, including
// initializers that need relocation.
static __thread uint32_t prototype_tls = 123;
static uint32_t prototype_tls_target = 7;
// volatile keeps the pointer, and so its relocated initializer, in the
// TLS image.
static __thread uint32_t *volatile prototype_tls_pointer = &prototype_tls_target;

static int
prototype_tls_fresh(void)
{
	return prototype_tls == 123 && prototype_tls_pointer == &prototype_tls_target && *prototype_tls_pointer == 7;
}

// A constructor runs on the initial thread before Go starts.
static int prototype_tls_constructor_ok;

__attribute__((constructor)) static void
prototype_tls_constructor(void)
{
	prototype_tls_constructor_ok = prototype_tls_fresh();
}

uint32_t
prototype_tls_constructor_passed(void)
{
	return prototype_tls_constructor_ok;
}

uint32_t
prototype_tls_bump(void)
{
	return ++prototype_tls;
}

static void *
prototype_tls_thread(void *opaque)
{
	uint32_t *seen = opaque;
	seen[0] = prototype_tls_fresh() ? prototype_tls : 0;
	seen[1] = prototype_tls_bump();
	return 0;
}

uint32_t
prototype_thread_tls(void)
{
	pthread_t thread;
	uint32_t seen[2] = { 0, 0 };

	if (pthread_create(&thread, 0, prototype_tls_thread, seen) != 0)
		return UINT32_MAX;
	if (pthread_join(thread, 0) != 0)
		return UINT32_MAX;
	return seen[0] == 123 && seen[1] == 124 ? 0 : seen[0] << 16 | seen[1];
}

// A pthread-specific value whose destructor counts threads that exit
// through pthread_exit, as a retiring Go M must.
static pthread_key_t prototype_retire_key;
static uint32_t prototype_retired_count;

static void
prototype_retire_destructor(void *value)
{
	__atomic_fetch_add(&prototype_retired_count, 1, __ATOMIC_SEQ_CST);
}

uint32_t
prototype_retire_init(void)
{
	return pthread_key_create(&prototype_retire_key, prototype_retire_destructor) == 0;
}

void
prototype_retire_mark(void)
{
	pthread_setspecific(prototype_retire_key, (void *)1);
}

uint32_t
prototype_retired(void)
{
	return __atomic_load_n(&prototype_retired_count, __ATOMIC_SEQ_CST);
}

struct prototype_callback_args {
	uint32_t value;
	uint32_t result;
};

static void *
prototype_callback_thread(void *opaque)
{
	struct prototype_callback_args *args = opaque;
	args->result = GoCallback(args->value);
	return 0;
}

uint32_t
prototype_thread_call_go(uint32_t value)
{
	pthread_t thread;
	struct prototype_callback_args args = { value, 0 };

	if (pthread_create(&thread, 0, prototype_callback_thread, &args) != 0)
		return UINT32_MAX;
	if (pthread_join(thread, 0) != 0)
		return UINT32_MAX;
	return args.result + 1;
}
