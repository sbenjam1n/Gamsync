/* telomere_registry.c — Transform registry (linked-list) */

#include "telomere_transform.h"
#include <string.h>

static t_transform_entry *registry_head = NULL;

void telomere_register_transform(t_symbol *name, t_transform_fn fn,
                                 const char *description,
                                 int min_args, int max_args)
{
    /* Check for existing entry with the same name */
    t_transform_entry *cur = registry_head;
    while (cur) {
        if (cur->name == name) {
            post("telomere: replacing transform '%s'", name->s_name);
            cur->fn          = fn;
            cur->description = description;
            cur->min_args    = min_args;
            cur->max_args    = max_args;
            return;
        }
        cur = cur->next;
    }

    /* Allocate and prepend new entry */
    t_transform_entry *entry = (t_transform_entry *)getbytes(sizeof(t_transform_entry));
    entry->name        = name;
    entry->fn          = fn;
    entry->description = description;
    entry->min_args    = min_args;
    entry->max_args    = max_args;
    entry->next        = registry_head;
    registry_head      = entry;
}

t_transform_entry *telomere_lookup_transform(t_symbol *name) {
    t_transform_entry *cur = registry_head;
    while (cur) {
        if (cur->name == name) return cur;
        cur = cur->next;
    }
    return NULL;
}

t_transform_entry *telomere_get_registry_head(void) {
    return registry_head;
}

void telomere_registry_free(void) {
    t_transform_entry *cur = registry_head;
    while (cur) {
        t_transform_entry *next = cur->next;
        freebytes(cur, sizeof(t_transform_entry));
        cur = next;
    }
    registry_head = NULL;
}
