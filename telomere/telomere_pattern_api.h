/* telomere_pattern_api.h — Pattern manipulation API for transforms */
#ifndef TELOMERE_PATTERN_API_H
#define TELOMERE_PATTERN_API_H

#include "telomere.h"

/* --- Read access --- */
int      pattern_num_events(t_telomere *x);
t_float  pattern_get_event(t_telomere *x, int index);
t_float *pattern_get_buffer(t_telomere *x);

/* --- Write access --- */
void     pattern_set_event(t_telomere *x, int index, t_float value);
void     pattern_append_event(t_telomere *x, t_float value);
void     pattern_resize(t_telomere *x, int new_size);
void     pattern_clear(t_telomere *x);
void     pattern_sort(t_telomere *x);

/* --- Bulk operations --- */
void     pattern_replace(t_telomere *x, t_float *new_data, int count);
void     pattern_copy_to(t_telomere *x, t_float *dest, int *count);

/* --- State queries --- */
t_float  pattern_get_quantize_pct(t_telomere *x);
int      pattern_get_grid(t_telomere *x);
t_float  pattern_get_tempo(t_telomere *x);

#endif /* TELOMERE_PATTERN_API_H */
