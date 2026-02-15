/* rotate.c — Cyclically shift pattern start point by N positions */

#include "../telomere_transform.h"
#include "../telomere_pattern_api.h"

static void transform_rotate(t_telomere *x, int argc, t_atom *argv) {
    int n = pattern_num_events(x);
    if (n == 0) return;

    int offset = (int)atom_getfloatarg(0, argc, argv);

    /* Normalize offset to grid-based fraction */
    int grid = pattern_get_grid(x);
    t_float shift = (t_float)offset / (t_float)grid;

    for (int i = 0; i < n; i++) {
        t_float val = pattern_get_event(x, i) + shift;
        /* Wrap into 0.0–1.0 */
        while (val >= 1.0f) val -= 1.0f;
        while (val < 0.0f)  val += 1.0f;
        pattern_set_event(x, i, val);
    }

    pattern_sort(x);
}

void rotate_register(void) {
    telomere_register_transform(
        gensym("rotate"),
        transform_rotate,
        "Cyclically shift pattern start point by N positions",
        1, 1
    );
}
