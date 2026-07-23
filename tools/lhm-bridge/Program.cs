// lhm-bridge: tiny stdout JSON streamer over LibreHardwareMonitorLib.
// Open Monitoring spawns this hidden and reads one JSON object per line.
// Sensor coverage depends on privileges: run elevated for CPU temperatures.

using System;
using System.Collections.Generic;
using System.Diagnostics;
using System.Text.Json;
using System.Threading;
using LibreHardwareMonitor.Hardware;

var computer = new Computer
{
    IsCpuEnabled = true,
    IsGpuEnabled = true,
    IsMemoryEnabled = true,
    IsStorageEnabled = true,
    IsMotherboardEnabled = true,
    IsControllerEnabled = true,
};

computer.Open();
var visitor = new UpdateVisitor();

var intervalMs = 2000;
if (args.Length > 0 && int.TryParse(args[0], out var ms) && ms >= 500)
    intervalMs = ms;

// Watch the parent process so we never outlive it (a broken stdout pipe is
// not always detected promptly on Windows).
Process parent = null;
if (args.Length > 1 && int.TryParse(args[1], out var parentPid) && parentPid > 0)
{
    try { parent = Process.GetProcessById(parentPid); } catch { return; }
}

var jsonOptions = new JsonSerializerOptions { PropertyNamingPolicy = JsonNamingPolicy.CamelCase };

// Exit when the parent (Open Monitoring) closes the pipe or dies.
while (true)
{
    if (parent != null && parent.HasExited)
        break;

    computer.Accept(visitor);

    var hw = new List<object>();
    foreach (IHardware hardware in computer.Hardware)
    {
        Collect(hardware, hw);
        foreach (IHardware sub in hardware.SubHardware)
            Collect(sub, hw, hardware.HardwareType.ToString());
    }

    try
    {
        Console.WriteLine(JsonSerializer.Serialize(new { hw }, jsonOptions));
        Console.Out.Flush();
    }
    catch (System.IO.IOException)
    {
        break; // parent went away
    }

    Thread.Sleep(intervalMs);
}

computer.Close();

static void Collect(IHardware hardware, List<object> into, string parentType = null)
{
    var sensors = new List<object>();
    foreach (ISensor s in hardware.Sensors)
    {
        if (!s.Value.HasValue || float.IsNaN(s.Value.Value)) continue;
        sensors.Add(new { t = s.SensorType.ToString(), n = s.Name, v = s.Value.Value });
    }
    if (sensors.Count > 0)
        into.Add(new { type = parentType ?? hardware.HardwareType.ToString(), name = hardware.Name, sensors });
}

class UpdateVisitor : IVisitor
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
