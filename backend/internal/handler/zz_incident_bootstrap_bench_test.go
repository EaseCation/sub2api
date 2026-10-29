package handler

import (
 "bytes"
 "fmt"
 "testing"
)

func BenchmarkIncidentBootstrapNormalization(b *testing.B) {
 for _, size := range []int{1<<20, 10<<20, 32<<20} {
  for _, shape := range []string{"plain_text", "base64_image", "delegation_with_history"} {
   b.Run(fmt.Sprintf("%s/%dMiB", shape, size>>20), func(b *testing.B) {
    var body []byte
    switch shape {
    case "base64_image":
     body=append([]byte(`{"model":"gpt-6-astra","input":[{"type":"message","role":"user","content":[{"type":"input_image","image_url":"data:image/png;base64,`), bytes.Repeat([]byte{'A'},size)...)
     body=append(body,[]byte(`"}]}]}`)...)
    default:
     body=append([]byte(`{"model":"gpt-6-astra","input":[{"type":"message","role":"user","content":"`),bytes.Repeat([]byte{'x'},size)...)
     if shape=="delegation_with_history" {
      body=append(body,[]byte(`"},{"type":"function_call_output","namespace":"codex_app","name":"send_message_to_thread","output":"<codex_delegation><source_thread_id>thread-1</source_thread_id><input>do the work</input></codex_delegation>"}]}`)...)
     } else { body=append(body,[]byte(`"}]}`)...)}
    }
    b.ReportAllocs();b.SetBytes(int64(len(body)));b.ResetTimer()
    for i:=0;i<b.N;i++ {
     out,_:=normalizeCodexAutomationBootstrap(body)
     out,changed:=normalizeCodexDelegationBootstrap(out)
     if changed!=(shape=="delegation_with_history") || len(out)==0 {b.Fatal("unexpected normalization result")}
    }
   })
  }
 }
}
