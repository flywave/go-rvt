//Headless: symbol substring search + decompile matches
//args: <outDir> <pattern1> [pattern2...]
import java.io.*;
import java.nio.file.*;
import java.util.*;

import ghidra.app.decompiler.*;
import ghidra.app.script.GhidraScript;
import ghidra.program.model.address.Address;
import ghidra.program.model.listing.*;
import ghidra.program.model.symbol.*;

public class SymDecomp extends GhidraScript {
    @Override
    protected void run() throws Exception {
        String[] args = getScriptArgs();
        File outDir = new File(args[0]);
        outDir.mkdirs();
        List<String> pats = new ArrayList<>();
        for (int i = 1; i < args.length; i++) pats.add(args[i]);

        Program prog = currentProgram;
        SymbolTable st = prog.getSymbolTable();
        Listing listing = prog.getListing();

        Map<String, List<Symbol>> hits = new LinkedHashMap<>();
        for (String p : pats) hits.put(p, new ArrayList<>());

        SymbolIterator it = st.getAllSymbols(true);
        int total = 0;
        while (it.hasNext() && !monitor.isCancelled()) {
            Symbol s = it.next();
            total++;
            String n = s.getName(true);
            if (n == null) continue;
            for (String p : pats) {
                if (n.contains(p)) hits.get(p).add(s);
            }
        }
        println("TOTAL_SYMBOLS=" + total);
        DecompInterface di = new DecompInterface();
        DecompileOptions opts = new DecompileOptions();
        di.setOptions(opts);
        di.openProgram(prog);

        for (Map.Entry<String, List<Symbol>> e : hits.entrySet()) {
            String p = e.getKey();
            println("PATTERN=" + p + " HITS=" + e.getValue().size());
            // unique addresses
            Set<Address> addrs = new TreeSet<>();
            for (Symbol s : e.getValue()) addrs.add(s.getAddress());
            int idx = 0;
            for (Address a : addrs) {
                if (monitor.isCancelled()) break;
                Function f = listing.getFunctionAt(a);
                if (f == null) f = listing.getFunctionContaining(a);
                String label;
                if (f != null) {
                    label = f.getName();
                } else {
                    label = "noFunc@" + a;
                }
                idx++;
                StringBuilder sb = new StringBuilder();
                sb.append("// pattern=").append(p).append(" addr=").append(a)
                  .append(" func=").append(label).append("\n");
                if (f != null) {
                    DecompileResults r = di.decompileFunction(f, 90, monitor);
                    if (r != null && r.decompileCompleted()) {
                        sb.append(r.getDecompiledFunction().getC());
                    } else {
                        sb.append("// DECOMPILE_FAILED: ").append(r == null ? "null" : r.getErrorMessage());
                    }
                } else {
                    sb.append("// NO_FUNCTION_BOUNDARY");
                }
                String safe = (p + "__" + label + "__" + a).replaceAll("[^A-Za-z0-9_@.\\-]", "_");
                if (safe.length() > 180) safe = safe.substring(0, 180);
                File fOut = new File(outDir, safe + ".c");
                Files.write(fOut.toPath(), sb.toString().getBytes("UTF-8"));
            }
            println("PATTERN=" + p + " WROTE=" + idx);
        }
        di.dispose();
        println("DONE");
    }
}
