// lhm-bridge — the hardware-sensor half of Open Monitoring.
//
// LibreHardwareMonitor is a C# library with no Go equivalent: it wraps NVAPI,
// AMD ADL, Intel IGCL, SMBIOS and the PawnIO kernel driver to reach sensors
// Windows exposes through no public API. Rather than reimplement that, the Go
// app embeds this worker, spawns it hidden, and reads one JSON object per line
// from its stdout.
//
// This process owns all knowledge of *sensor names*. LibreHardwareMonitor
// reports a flat, vendor-specific list ("CPU Package", "Core (Tctl/Tdie)",
// "GPU Hot Spot", ...); picking the right one per vendor belongs next to the
// sensor objects, not in the Go consumer. So the output here is already
// reduced to the fixed shape the app actually charts.
//
// Usage: lhm-bridge.exe [intervalMs] [parentPid]
//
// Privileges: CPU package temperature and power need the PawnIO driver
// (https://pawnio.eu) — when it is absent those fields are simply null and
// "pawnIo" reports false, which the app surfaces in Settings. Drive SMART is
// deliberately not read here: the app gets it from WMI without a helper.

using System;
using System.Collections.Generic;
using System.Diagnostics;
using System.Globalization;
using System.Linq;
using System.Text.Json;
using System.Text.Json.Serialization;
using System.Threading;
using LibreHardwareMonitor.Hardware;

const int defaultIntervalMs = 2000;
const int minIntervalMs = 500;

int intervalMs = args.Length > 0 && int.TryParse(args[0], NumberStyles.Integer, CultureInfo.InvariantCulture, out int ms) && ms >= minIntervalMs
    ? ms
    : defaultIntervalMs;

// Watch the parent process so this worker never outlives the app. A broken
// stdout pipe is not always reported promptly on Windows, so the PID check is
// the reliable signal; the IOException below is the fast path.
Process? parent = null;
if (args.Length > 1 && int.TryParse(args[1], NumberStyles.Integer, CultureInfo.InvariantCulture, out int parentPid) && parentPid > 0)
{
    try { parent = Process.GetProcessById(parentPid); }
    catch (ArgumentException) { return; } // parent already gone
}

var computer = new Computer
{
    IsCpuEnabled = true,
    IsGpuEnabled = true,
    IsMemoryEnabled = true,
    IsMotherboardEnabled = true,
};

computer.Open();

var visitor = new UpdateVisitor();
var json = new JsonSerializerOptions
{
    PropertyNamingPolicy = JsonNamingPolicy.CamelCase,
    DefaultIgnoreCondition = JsonIgnoreCondition.WhenWritingNull,
};

try
{
    while (parent is not { HasExited: true })
    {
        computer.Accept(visitor);

        try
        {
            Console.WriteLine(JsonSerializer.Serialize(Reading.Capture(computer), json));
            Console.Out.Flush();
        }
        catch (System.IO.IOException)
        {
            break; // the app closed the pipe
        }

        Thread.Sleep(intervalMs);
    }
}
finally
{
    computer.Close();
}

/// <summary>One sampled line of output. Null means "sensor not available".</summary>
internal sealed class Reading
{
    /// <summary>Whether the PawnIO driver is installed; gates CPU temperature and power.</summary>
    public bool PawnIo { get; init; }

    public string? PawnIoVersion { get; init; }

    public CpuReading? Cpu { get; init; }

    public GpuReading? Gpu { get; init; }

    /// <summary>Static machine description from SMBIOS; unchanging, but cheap to resend.</summary>
    public SystemReading? System { get; init; }

    // Takes the concrete Computer rather than IComputer: SMBios is only exposed
    // on the implementation.
    public static Reading Capture(Computer computer)
    {
        List<IHardware> hardware = Flatten(computer.Hardware).ToList();

        return new Reading
        {
            PawnIo = LibreHardwareMonitor.PawnIo.PawnIo.IsInstalled,
            PawnIoVersion = LibreHardwareMonitor.PawnIo.PawnIo.Version?.ToString(),
            System = SystemReading.From(computer.SMBios),
            Cpu = hardware.Where(h => h.HardwareType == HardwareType.Cpu)
                          .Select(CpuReading.From)
                          .FirstOrDefault(),
            Gpu = PickGpu(hardware),
        };
    }

    /// <summary>
    /// Chooses the GPU to report. A laptop or a CPU with integrated graphics
    /// exposes several; the discrete card is the one worth charting, so
    /// integrated GPUs are only used when nothing else is present.
    /// </summary>
    private static GpuReading? PickGpu(IEnumerable<IHardware> hardware)
    {
        IHardware[] gpus = hardware
            .Where(h => h.HardwareType is HardwareType.GpuNvidia or HardwareType.GpuAmd or HardwareType.GpuIntel)
            .OrderBy(h => h.HardwareType == HardwareType.GpuIntel ? 1 : 0)
            .ToArray();

        return gpus.Select(GpuReading.From).FirstOrDefault(g => g.HasData);
    }

    /// <summary>Hardware plus its sub-hardware, which is where SuperIO sensors live.</summary>
    private static IEnumerable<IHardware> Flatten(IEnumerable<IHardware> hardware)
    {
        foreach (IHardware h in hardware)
        {
            yield return h;

            foreach (IHardware sub in Flatten(h.SubHardware))
                yield return sub;
        }
    }
}

/// <summary>
/// The machine description SMBIOS provides. Reading it needs no privileges and
/// no driver, which is why it comes from here rather than from WMI.
/// </summary>
internal sealed class SystemReading
{
    public string? Board { get; init; }

    public MemoryReading? Memory { get; init; }

    public static SystemReading? From(SMBios smbios)
    {
        if (smbios is null)
            return null;

        string board = Join(smbios.Board?.ManufacturerName, smbios.Board?.ProductName);

        return new SystemReading
        {
            Board = string.IsNullOrEmpty(board) ? null : board,
            Memory = MemoryReading.From(smbios.MemoryDevices),
        };
    }

    private static string Join(string? a, string? b) =>
        string.Join(" ", new[] { a, b }.Where(s => !string.IsNullOrWhiteSpace(s))).Trim();
}

internal sealed class MemoryReading
{
    /// <summary>Populated slots, not total slots.</summary>
    public int Modules { get; init; }

    public double ModuleGb { get; init; }

    public int SpeedMt { get; init; }

    public string? Type { get; init; }

    public string? Vendor { get; init; }

    public static MemoryReading? From(MemoryDevice[]? devices)
    {
        // Boards report every slot, including the empty ones — those come back
        // with a size of zero and would otherwise be counted as modules.
        MemoryDevice[] populated = (devices ?? Array.Empty<MemoryDevice>())
            .Where(d => d.Size > 0)
            .ToArray();

        if (populated.Length == 0)
            return null;

        MemoryDevice first = populated[0];

        return new MemoryReading
        {
            Modules = populated.Length,
            ModuleGb = first.Size / 1024.0, // SMBIOS reports megabytes
            SpeedMt = first.ConfiguredSpeed > 0 ? first.ConfiguredSpeed : first.Speed,
            Type = first.Type.ToString(),
            Vendor = string.IsNullOrWhiteSpace(first.ManufacturerName) ? null : first.ManufacturerName.Trim(),
        };
    }
}

internal sealed class CpuReading
{
    public double? TempC { get; init; }

    public double? PowerW { get; init; }

    public double? ClockMhz { get; init; }

    public static CpuReading From(IHardware cpu) => new()
    {
        // Intel reports "CPU Package"; AMD reports "Core (Tctl/Tdie)". The
        // remaining names are fallbacks for older or unusual parts.
        TempC = Sensors.Pick(cpu, SensorType.Temperature,
                             "CPU Package", "Core (Tctl/Tdie)", "Core Max", "Core Average")
                ?? Sensors.Max(cpu, SensorType.Temperature, "Core"),
        PowerW = Sensors.Pick(cpu, SensorType.Power, "CPU Package", "Package"),
        // The fastest core, not the average: on hybrid CPUs an average across
        // P- and E-cores matches neither, and overlays conventionally show peak.
        ClockMhz = Sensors.Max(cpu, SensorType.Clock, "Core"),
    };
}

internal sealed class GpuReading
{
    public string? Name { get; init; }

    public double? Usage { get; init; }

    public double? TempC { get; init; }

    /// <summary>Hot spot / junction temperature, where the vendor exposes it.</summary>
    public double? HotspotC { get; init; }

    public double? PowerW { get; init; }

    public double? CoreMhz { get; init; }

    public double? MemMhz { get; init; }

    public double? MemUsedMb { get; init; }

    public double? MemTotalMb { get; init; }

    public double? FanPercent { get; init; }

    [JsonIgnore]
    public bool HasData => Usage is > 0 || TempC is > 0 || MemTotalMb is > 0;

    public static GpuReading From(IHardware gpu) => new()
    {
        Name = gpu.Name,
        Usage = Sensors.Pick(gpu, SensorType.Load, "GPU Core"),
        TempC = Sensors.Pick(gpu, SensorType.Temperature, "GPU Core"),
        HotspotC = Sensors.Pick(gpu, SensorType.Temperature, "GPU Hot Spot", "GPU Memory Junction"),
        PowerW = Sensors.Pick(gpu, SensorType.Power, "GPU Package", "GPU Power"),
        CoreMhz = Sensors.Pick(gpu, SensorType.Clock, "GPU Core"),
        MemMhz = Sensors.Pick(gpu, SensorType.Clock, "GPU Memory"),
        MemUsedMb = Sensors.Pick(gpu, SensorType.SmallData, "GPU Memory Used"),
        MemTotalMb = Sensors.Pick(gpu, SensorType.SmallData, "GPU Memory Total"),
        // Control is the fan duty cycle in percent; SensorType.Fan would be RPM.
        FanPercent = Sensors.Pick(gpu, SensorType.Control, "GPU Fan"),
    };
}

/// <summary>Name-based lookup over one hardware node's sensor list.</summary>
internal static class Sensors
{
    /// <summary>
    /// First readable sensor of the given type whose name contains one of the
    /// candidates, tried in order of preference. Null when none match.
    /// </summary>
    public static double? Pick(IHardware hardware, SensorType type, params string[] candidates)
    {
        foreach (string candidate in candidates)
        {
            foreach (ISensor sensor in hardware.Sensors)
            {
                if (sensor.SensorType == type &&
                    sensor.Name.Contains(candidate, StringComparison.OrdinalIgnoreCase) &&
                    Readable(sensor) is { } value)
                {
                    return value;
                }
            }
        }

        return null;
    }

    /// <summary>Highest reading among same-type sensors matching a name fragment.</summary>
    public static double? Max(IHardware hardware, SensorType type, string contains)
    {
        double? max = null;

        foreach (ISensor sensor in hardware.Sensors)
        {
            if (sensor.SensorType == type &&
                sensor.Name.Contains(contains, StringComparison.OrdinalIgnoreCase) &&
                Readable(sensor) is { } value &&
                (max is null || value > max))
            {
                max = value;
            }
        }

        return max;
    }

    /// <summary>
    /// A sensor's value, or null when it is missing or nonsensical. Sensors
    /// backed by an absent driver report exactly 0, which is indistinguishable
    /// from a real zero — treating it as "no reading" is what keeps the UI
    /// showing "—" instead of a confident 0 °C.
    /// </summary>
    private static double? Readable(ISensor sensor)
    {
        float? value = sensor.Value;

        if (value is null || float.IsNaN(value.Value) || float.IsInfinity(value.Value) || value.Value <= 0)
            return null;

        return Math.Round(value.Value, 1);
    }
}

/// <summary>Refreshes every hardware node before its sensors are read.</summary>
internal sealed class UpdateVisitor : IVisitor
{
    public void VisitComputer(IComputer computer) => computer.Traverse(this);

    public void VisitHardware(IHardware hardware)
    {
        hardware.Update();

        foreach (IHardware sub in hardware.SubHardware)
            sub.Accept(this);
    }

    public void VisitSensor(ISensor sensor) { }

    public void VisitParameter(IParameter parameter) { }
}
