/* telomere_pattern_api.c — Pattern manipulation API implementation */

#include "telomere_pattern_api.h"
#include <string.h>

/* --- Read access --- */

int pattern_num_events(t_telomere *x) {
    return x->num_events;
}

t_float pattern_get_event(t_telomere *x, int index) {
    if (index < 0 || index >= x->num_events) return 0.0f;
    return x->pattern[index];
}

t_float *pattern_get_buffer(t_telomere *x) {
    return x->pattern;
}

/* --- Write access --- */

void pattern_set_event(t_telomere *x, int index, t_float value) {
    if (index < 0 || index >= x->num_events) return;
    /* Clamp to 0.0–1.0 range */
    if (value < 0.0f) value = 0.0f;
    if (value > 1.0f) value = 1.0f;
    x->pattern[index] = value;
}

static void pattern_ensure_capacity(t_telomere *x, int needed) {
    if (needed <= x->pattern_alloc) return;
    int new_alloc = x->pattern_alloc;
    while (new_alloc < needed) {
        new_alloc = (new_alloc < 16) ? 16 : new_alloc * 2;
    }
    if (new_alloc > TELOMERE_MAX_EVENTS) new_alloc = TELOMERE_MAX_EVENTS;
    x->pattern = (t_float *)resizebytes(x->pattern,
        x->pattern_alloc * sizeof(t_float),
        new_alloc * sizeof(t_float));
    x->pattern_alloc = new_alloc;
}

void pattern_append_event(t_telomere *x, t_float value) {
    if (x->num_events >= TELOMERE_MAX_EVENTS) return;
    if (value < 0.0f) value = 0.0f;
    if (value > 1.0f) value = 1.0f;
    pattern_ensure_capacity(x, x->num_events + 1);
    x->pattern[x->num_events++] = value;
}

void pattern_resize(t_telomere *x, int new_size) {
    if (new_size < 0) new_size = 0;
    if (new_size > TELOMERE_MAX_EVENTS) new_size = TELOMERE_MAX_EVENTS;
    pattern_ensure_capacity(x, new_size);
    /* Zero-fill any newly added slots */
    if (new_size > x->num_events) {
        memset(x->pattern + x->num_events, 0,
               (new_size - x->num_events) * sizeof(t_float));
    }
    x->num_events = new_size;
}

void pattern_clear(t_telomere *x) {
    x->num_events = 0;
}

/* Simple insertion sort — patterns are small */
void pattern_sort(t_telomere *x) {
    int n = x->num_events;
    t_float *p = x->pattern;
    for (int i = 1; i < n; i++) {
        t_float key = p[i];
        int j = i - 1;
        while (j >= 0 && p[j] > key) {
            p[j + 1] = p[j];
            j--;
        }
        p[j + 1] = key;
    }
}

/* --- Bulk operations --- */

void pattern_replace(t_telomere *x, t_float *new_data, int count) {
    if (count < 0) count = 0;
    if (count > TELOMERE_MAX_EVENTS) count = TELOMERE_MAX_EVENTS;
    pattern_ensure_capacity(x, count);
    memcpy(x->pattern, new_data, count * sizeof(t_float));
    x->num_events = count;
}

void pattern_copy_to(t_telomere *x, t_float *dest, int *count) {
    memcpy(dest, x->pattern, x->num_events * sizeof(t_float));
    *count = x->num_events;
}

/* --- State queries --- */

t_float pattern_get_quantize_pct(t_telomere *x) {
    return x->quantize_pct;
}

int pattern_get_grid(t_telomere *x) {
    return x->grid;
}

t_float pattern_get_tempo(t_telomere *x) {
    return x->tempo;
}
