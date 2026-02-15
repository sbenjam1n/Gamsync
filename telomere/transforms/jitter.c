/* jitter.c — Apply random displacement to event positions */

#include "../telomere_transform.h"
#include "../telomere_pattern_api.h"
#include <stdlib.h>

static void transform_jitter(t_telomere *x, int argc, t_atom *argv) {
    int n = pattern_num_events(x);
    if (n == 0) return;

    t_float amount = atom_getfloatarg(0, argc, argv);
    if (amount < 0.0f) amount = 0.0f;
    if (amount > 1.0f) amount = 1.0f;

    for (int i = 0; i < n; i++) {
        t_float val = pattern_get_event(x, i);
        /* Random displacement in range [-amount, +amount] */
        float r = ((float)rand() / (float)RAND_MAX) * 2.0f - 1.0f;
        val += r * amount;
        /* Wrap into 0.0–1.0 */
        while (val >= 1.0f) val -= 1.0f;
        while (val < 0.0f)  val += 1.0f;
        pattern_set_event(x, i, val);
    }

    pattern_sort(x);
}

void jitter_register(void) {
    telomere_register_transform(
        gensym("jitter"),
        transform_jitter,
        "Apply random displacement to event positions",
        1, 1
    );
}
