#ifndef UTUIMP_SINK_WINDOWS_H
#define UTUIMP_SINK_WINDOWS_H

#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

typedef void* utuimp_winsink_t;

typedef void (*utuimp_winsink_eos_cb_t)(uintptr_t user_data);
typedef void (*utuimp_winsink_err_cb_t)(uintptr_t user_data, char* err_msg);
typedef void (*utuimp_winsink_stream_start_cb_t)(uintptr_t user_data, char* uri_utf8);

// utuimp_winsink_create returns a sink that decodes the ADTS AAC stream at a
// loopback URL with Media Foundation and plays the PCM through waveOut.
utuimp_winsink_t utuimp_winsink_create(
    uintptr_t user_data,
    utuimp_winsink_eos_cb_t on_eos,
    utuimp_winsink_err_cb_t on_err,
    utuimp_winsink_stream_start_cb_t on_stream_start);
void utuimp_winsink_play_uri(utuimp_winsink_t handle, const char* uri_utf8);
void utuimp_winsink_set_next_uri(utuimp_winsink_t handle, const char* uri_utf8);
void utuimp_winsink_play(utuimp_winsink_t handle);
void utuimp_winsink_pause(utuimp_winsink_t handle);
void utuimp_winsink_stop(utuimp_winsink_t handle);
void utuimp_winsink_set_volume(utuimp_winsink_t handle, double volume);
// utuimp_winsink_set_eq sets gains in dB for the 10 bands, lowest first.
void utuimp_winsink_set_eq(utuimp_winsink_t handle, const double* gains_db, int count);
// utuimp_winsink_position returns seconds of audio played since the last play_uri.
double utuimp_winsink_position(utuimp_winsink_t handle);
int utuimp_winsink_read_tap(utuimp_winsink_t handle, float* dst, int n, double* ahead, int64_t* end);
void utuimp_winsink_destroy(utuimp_winsink_t handle);

#ifdef __cplusplus
}
#endif

#endif // UTUIMP_SINK_WINDOWS_H
