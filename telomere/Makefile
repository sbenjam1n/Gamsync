# Makefile for telomere Pd external
#
# Usage:
#   make              — build telomere.pd_linux (or .pd_darwin on macOS)
#   make PD_PATH=/usr/include/pd   — specify Pd headers location
#   make clean        — remove build artifacts
#   make test         — compile-check with stub headers

# Detect platform
UNAME := $(shell uname -s)
ifeq ($(UNAME),Darwin)
  SUFFIX = pd_darwin
  LDFLAGS = -bundle -undefined dynamic_lookup
else
  SUFFIX = pd_linux
  LDFLAGS = -shared -Wl,--export-dynamic
endif

# Pure Data header path (override with: make PD_PATH=/path/to/pd)
PD_PATH ?= pd

CC = gcc
CFLAGS = -Wall -Wextra -Wpedantic -O2 -fPIC -I$(PD_PATH)
TARGET = telomere.$(SUFFIX)

# Source files
CORE_SRC = telomere.c \
           telomere_pattern_api.c \
           telomere_registry.c

TRANSFORM_SRC = transforms/builtins.c \
                transforms/palindrome.c \
                transforms/rotate.c \
                transforms/reverse.c \
                transforms/fast.c \
                transforms/slow.c \
                transforms/euclid.c \
                transforms/jitter.c \
                transforms/skip.c \
                transforms/degrade.c

ALL_SRC = $(CORE_SRC) $(TRANSFORM_SRC)
ALL_OBJ = $(ALL_SRC:.c=.o)

# Default target
all: $(TARGET)

$(TARGET): $(ALL_OBJ)
	$(CC) $(LDFLAGS) -o $@ $^

%.o: %.c
	$(CC) $(CFLAGS) -c -o $@ $<

# Test target: compile with the bundled stub headers
test:
	$(MAKE) PD_PATH=pd all

clean:
	rm -f $(ALL_OBJ) $(TARGET) telomere.pd_linux telomere.pd_darwin

.PHONY: all clean test
