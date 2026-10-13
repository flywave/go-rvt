//args: <outFile> <addr1> <addr2> ...  — dump instructions + xrefs around each addr
import java.io.*;
import java.nio.file.*;
import java.util.*;

import ghidra.app.script.GhidraScript;
import ghidra.program.model.address.Address;
import ghidra.program.model.listing.*;
import ghidra.program.model.symbol.*;

public class DumpAsm extends GhidraScript {
    @Override
    protected void run() throws Exception {
        String[] args = getScriptArgs();
        StringBuilder sb = new StringBuilder();
        Program prog = currentProgram;
        Listing listing = prog.getListing();
        SymbolTable st = prog.getSymbolTable();
        for (int i = 1; i < args.length; i++) {
            Address a = prog.getAddressFactory().getDefaultAddressSpace().getAddress(args[i]);
            sb.append("=== ADDR ").append(a).append(" ===\n");
            // symbols at address
            for (Symbol s : st.getSymbols(a)) {
                sb.append("SYM: ").append(s.getName(true)).append(" type=").append(s.getSymbolType()).append("\n");
            }
            Function f = listing.getFunctionAt(a);
            if (f == null) f = listing.getFunctionContaining(a);
            sb.append("FUNC: ").append(f == null ? "null" : (f.getName() + " @" + f.getEntryPoint() + " size=" + f.getBody().getNumAddresses())).append("\n");
            InstructionIterator it = listing.getInstructions(a, true);
            int n = 0;
            while (it.hasNext() && n < 80) {
                Instruction ins = it.next();
                sb.append(ins.getAddress()).append("  ").append(ins.toString()).append("\n");
                n++;
            }
            // xrefs to this address
            ghidra.program.model.symbol.ReferenceManager rm = prog.getReferenceManager();
            ghidra.program.model.symbol.ReferenceIterator ri = rm.getReferencesTo(a);
            int xr = 0;
            while (ri.hasNext() && xr < 20) {
                ghidra.program.model.symbol.Reference r = ri.next();
                Function cf = listing.getFunctionContaining(r.getFromAddress());
                sb.append("XREF_FROM: ").append(r.getFromAddress())
                  .append(" type=").append(r.getReferenceType())
                  .append(" inFn=").append(cf == null ? "-" : cf.getName()).append("\n");
                xr++;
            }
            sb.append("\n");
        }
        Files.write(Paths.get(args[0]), sb.toString().getBytes("UTF-8"));
        println("DumpAsm done");
    }
}
