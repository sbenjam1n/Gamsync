/* euclid.c — Replace pattern with a Euclidean rhythm distribution */

#include "../telomere_transform.h"
#include "../telomere_pattern_api.h"

/*
 * Bjorklund's algorithm: distribute k hits across n slots as evenly
 * as possible. Classic Euclidean rhythm generator.
 */
static void bjorklund(int *pattern, int n, int k) {
    if (k > n) k = n;
    if (k <= 0) { for (int i = 0; i < n; i++) pattern[i] = 0; return; }

    /* Initialize: k ones, (n-k) zeros */
    for (int i = 0; i < n; i++)
        pattern[i] = (i < k) ? 1 : 0;

    int divisor = n - k;
    int remainder = k;
    int level = 0;
    int counts[64], remainders_arr[64];

    remainders_arr[0] = remainder;

    while (remainder > 1) {
        counts[level] = divisor / remainder;
        remainders_arr[level + 1] = divisor % remainder;
        divisor = remainder;
        remainder = remainders_arr[level + 1];
        level++;
    }
    counts[level] = divisor;

    /* Build sequence from counts */
    int seq[256];
    int seq_len = 0;

    /* Reconstruct via the Bjorklund tree */
    /* Simplified: just use the computed counts to interleave */
    /* For correctness, use a direct rotation-based approach */
    for (int i = 0; i < n; i++) {
        /* Bresenham-style distribution */
        pattern[i] = 0;
    }

    /* Bresenham distribution of k hits in n slots */
    for (int i = 0; i < k; i++) {
        int pos = (i * n) / k;
        pattern[pos] = 1;
    }

    (void)seq; (void)seq_len; (void)counts;
}

static void transform_euclid(t_telomere *x, int argc, t_atom *argv) {
    int hits = (int)atom_getfloatarg(0, argc, argv);
    int steps = (int)atom_getfloatarg(1, argc, argv);

    if (steps < 1) steps = 1;
    if (steps > 64) steps = 64;
    if (hits < 0) hits = 0;
    if (hits > steps) hits = steps;

    int rhythm[64];
    bjorklund(rhythm, steps, hits);

    /* Replace pattern with euclidean positions */
    pattern_clear(x);
    t_float step_size = 1.0f / (t_float)steps;

    for (int i = 0; i < steps; i++) {
        if (rhythm[i]) {
            pattern_append_event(x, step_size * (t_float)i);
        }
    }

    pattern_sort(x);
}

void euclid_register(void) {
    telomere_register_transform(
        gensym("euclid"),
        transform_euclid,
        "Replace pattern with Euclidean rhythm (hits steps)",
        2, 2
    );
}
