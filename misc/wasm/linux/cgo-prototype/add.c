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

static uint32_t
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
