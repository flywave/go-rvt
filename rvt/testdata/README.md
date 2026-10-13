# RVT Test Data

Regression test fixtures. These binary files are excluded from git (`.gitignore`);
place them here before running `go test ./rvt/`.

| File | Revit Version | Size | Source |
|---|---|---|---|
| `rac_basic_2023.rvt` | 2023 | 18.9 MB | Autodesk sample (architecture basic) |
| `rst_advanced_2022.rvt` | 2022 | 14.4 MB | Autodesk sample (structure advanced) |
| `rac_advanced_2017.rvt` | 2017 | 15.3 MB | Autodesk-Forge/viewer-navigation.sample |

Download: copy from Revit install (`C:\Program Files\Autodesk\Revit <Year>\Samples`)
or from [Autodesk-Forge/viewer-navigation.sample](https://github.com/Autodesk-Forge/viewer-navigation.sample/tree/master/samples).

Tests skip gracefully when files are absent.
