# Nλvescript v3.0 "Singularity" Security Research Examples

This document demonstrates how to use the "Legitimate Security Research" features natively in Nλvescript v3.0.

## 1. Binary Analysis (ELF/PE)

```navescript
// Parse an ELF binary natively
let info = await Binary_parse("/bin/ls", "elf")
print("Arch:", info.arch)
print("Entry Point:", info.entry_point)

// Disassemble first 100 instructions
let code = await Binary_disassemble("/bin/ls", "x86")
for ins in code {
    print(ins.address, ":", ins.mnemonic, ins.operands)
}
```

## 2. Process Memory Forensics

```navescript
// Open a process by PID
let proc = await Process_open(1234)
if (proc != nil) {
    print("Process Name:", proc.name)
    print("Memory Usage:", proc.memory, "bytes")
    
    // Read 1024 bytes from an address
    let data = await Process_read(1234, 0x400000, 1024)
    print("Memory read successful, buffer length:", data.len())
}
```

## 3. Network Packet Capture

```navescript
// Capture 10 packets on eth0 with a filter
let packets = await Network_capture("eth0", "tcp port 80")
for packet in packets {
    print("Captured packet length:", packet.len())
}
```

## 4. Fuzzing & Mutation

```navescript
let original = Buffer([0xDE, 0xAD, 0xBE, 0xEF])

// Bitflip mutation
let fuzzed1 = await Fuzzer_mutate(original, "bitflip")
print("Bitflip:", fuzzed1)

// Byteswap mutation
let fuzzed2 = await Fuzzer_mutate(original, "byteswap")
print("Byteswap:", fuzzed2)
```

---

**Note:** These features are intended for legitimate security research on systems you own or are authorized to test. Use of these tools for unauthorized access is illegal.
