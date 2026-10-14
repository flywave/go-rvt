import subprocess, re, sys, struct

def load_elf_sections(path):
    data = open(path,'rb').read()
    # ELF64 header
    e_shoff = struct.unpack('<Q', data[0x28:0x30])[0]
    e_shentsize = struct.unpack('<H', data[0x3a:0x3c])[0]
    e_shnum = struct.unpack('<H', data[0x3c:0x3e])[0]
    e_shstrndx = struct.unpack('<H', data[0x3e:0x40])[0]
    def sh(i):
        o = e_shoff + i*e_shentsize
        (name, typ, flags, addr, off, size) = struct.unpack('<IIQQQQ', data[o:o+40])
        return name, typ, addr, off, size
    _,_,_,stroff,strsize = sh(e_shstrndx)
    def name_of(i):
        n = sh(i)[0]
        end = data.index(b'\0', stroff+n)
        return data[stroff+n:end].decode()
    secs = {}
    for i in range(e_shnum):
        nm, typ, addr, off, size = sh(i)
        secs[name_of(i)] = (addr, off, size)
    return data, secs

class Reader:
    def __init__(self, path):
        self.data, self.secs = load_elf_sections(path)
    def read_vma(self, vma, n):
        for name,(addr, off, size) in self.secs.items():
            if addr and addr <= vma < addr+size and name != '.bss':
                o = off + (vma-addr)
                return self.data[o:o+n]
        return None
    def u32str(self, vma):
        raw = self.read_vma(vma, 512)
        if not raw: return None
        chars = []
        for i in range(0, len(raw)-3, 4):
            cp = struct.unpack('<I', raw[i:i+4])[0]
            if cp == 0: break
            if 0x20 <= cp < 0x7f: chars.append(chr(cp))
            else: return None
        s = ''.join(chars)
        return s if len(s) >= 2 else None

def extract(r, module, addr, size=0x3000):
    out = subprocess.run(["objdump","-d",f"--start-address={hex(addr)}",f"--stop-address={hex(addr+size)}",module],capture_output=True,text=True).stdout
    props = []
    for line in out.splitlines():
        m = re.search(r'leaq\s+(-?0x[0-9a-f]+)\(%rip\),\s*%r[a-z0-9]+\s*#\s*0x([0-9a-f]+)', line)
        if m:
            s = r.u32str(int(m.group(2), 16))
            if s and re.match(r'^[A-Za-z_][A-Za-z0-9_ ]*$', s) and not s.startswith('_Z'):
                props.append(s)
    return props

if __name__ == '__main__':
    module = sys.argv[1]
    r = Reader(module)
    for sym in sys.argv[2:]:
        addr, name = sym.split('=')
        props = extract(r, module, int(addr,16))
        print(f"== {name} @ {addr}: {len(props)} properties ==")
        for i,p in enumerate(props):
            print(f"  {i:2d} {p}")
        print()
