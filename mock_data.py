"""
Mock Data Generator
Generates realistic FusionCompute inventory data for testing without a real connection.
"""

import random
import time


def generate_mock_data():
    """Generate a complete set of mock inventory data matching collector output format."""

    # ── Clusters ─────────────────────────────────────────
    clusters = ["CL-Production", "CL-Development", "CL-DMZ"]

    # ── Hosts ────────────────────────────────────────────
    host_models = [
        ("Huawei FusionServer 2288H V5", "Intel Xeon Gold 6248R", 2400, 48),
        ("Huawei FusionServer 2288H V6", "Intel Xeon Gold 6338", 2600, 64),
        ("Huawei FusionServer 2488H V5", "Intel Xeon Platinum 8280", 2700, 56),
        ("Huawei FusionServer RH2288 V3", "Intel Xeon E5-2680 v4", 2400, 28),
    ]

    hosts_data = []
    for i in range(8):
        model, cpu_model, mhz, cores = host_models[i % len(host_models)]
        cluster = clusters[i % len(clusters)]
        mem_total = random.choice([262144, 393216, 524288, 786432])  # MB
        mem_used = int(mem_total * random.uniform(0.4, 0.85))
        running_vms = random.randint(5, 25)

        hosts_data.append({
            "Host Name": f"esxi-host-{i+1:02d}",
            "IP Address": f"10.10.{10 + i // 4}.{11 + i}",
            "Status": "normal",
            "Cluster": cluster,
            "CPU Model": cpu_model,
            "CPU Cores": cores,
            "CPU MHz": mhz,
            "Memory Total (MB)": mem_total,
            "Memory Used (MB)": mem_used,
            "Running VMs": running_vms,
            "Maintenance Mode": False,
            "Hypervisor": "FusionCompute",
            "BMC IP": f"10.10.{20 + i // 4}.{11 + i}",
            "URN": f"urn:sites:1:hosts:{1000 + i}",
        })

    # ── Datastores ───────────────────────────────────────
    ds_types = ["SAN", "NAS", "FusionStorage", "Local"]
    datastores_data = []
    ds_names = [
        "DS-PROD-SAN-01", "DS-PROD-SAN-02", "DS-DEV-NAS-01",
        "DS-BACKUP-01", "DS-ISO-LOCAL", "DS-FUSIONSTORAGE-01",
    ]
    for i, name in enumerate(ds_names):
        cap = random.choice([2048, 4096, 8192, 10240, 20480])
        free = int(cap * random.uniform(0.15, 0.65))
        used_pct = round((1 - free / cap) * 100, 1)
        datastores_data.append({
            "Datastore Name": name,
            "Storage Type": ds_types[i % len(ds_types)],
            "Capacity (GB)": cap,
            "Free (GB)": free,
            "Used %": used_pct,
            "Status": "normal",
            "Thin Provisioning": True,
            "URN": f"urn:sites:1:datastores:{2000 + i}",
        })

    # ── Networks ─────────────────────────────────────────
    vswitch_data = []
    dvs_names = ["DVS-Management", "DVS-Production", "DVS-Storage"]
    pg_list = [
        ("PG-Mgmt-VLAN10", 10, "DVS-Management"),
        ("PG-Prod-VLAN100", 100, "DVS-Production"),
        ("PG-Prod-VLAN101", 101, "DVS-Production"),
        ("PG-Prod-VLAN102", 102, "DVS-Production"),
        ("PG-DMZ-VLAN200", 200, "DVS-Production"),
        ("PG-DB-VLAN300", 300, "DVS-Production"),
        ("PG-Storage-VLAN500", 500, "DVS-Storage"),
        ("PG-Backup-VLAN501", 501, "DVS-Storage"),
    ]

    for dvs in dvs_names:
        vswitch_data.append({
            "Name": dvs,
            "Type": "DVSwitch",
            "VLAN ID": "",
            "MTU": 1500,
            "Description": f"Distributed Virtual Switch - {dvs}",
            "URN": f"urn:sites:1:dvswitchs:{dvs}",
        })

    for pg_name, vlan, dvs_name in pg_list:
        vswitch_data.append({
            "Name": pg_name,
            "Type": "Port Group",
            "VLAN ID": vlan,
            "MTU": 1500,
            "Description": dvs_name,
            "URN": f"urn:sites:1:portgroups:{pg_name}",
        })

    # ── VMs ──────────────────────────────────────────────
    os_types = [
        "CentOS 7.9 64bit", "CentOS 8.5 64bit", "Ubuntu 20.04 64bit",
        "Ubuntu 22.04 64bit", "Windows Server 2019 64bit",
        "Windows Server 2022 64bit", "Red Hat 8.6 64bit",
        "SUSE Linux 15 SP4 64bit", "Debian 11 64bit",
    ]

    vm_names = [
        "web-server-01", "web-server-02", "web-server-03",
        "app-server-01", "app-server-02",
        "db-master-01", "db-replica-01", "db-replica-02",
        "redis-cache-01", "redis-cache-02",
        "nginx-lb-01", "nginx-lb-02",
        "monitor-grafana", "monitor-prometheus",
        "elk-logstash-01", "elk-elastic-01", "elk-kibana-01",
        "jenkins-ci", "gitlab-runner-01", "gitlab-runner-02",
        "dns-server-01", "ntp-server-01",
        "vpn-gateway-01", "mail-server-01",
        "file-server-01", "backup-agent-01",
        "k8s-master-01", "k8s-worker-01", "k8s-worker-02", "k8s-worker-03",
    ]

    power_states = ["running", "running", "running", "running", "stopped", "running"]

    vinfo = []
    vcpu = []
    vmemory = []
    vdisk = []
    vnetwork = []

    for i, vm_name in enumerate(vm_names):
        host = hosts_data[i % len(hosts_data)]
        cluster = host["Cluster"]
        os_type = os_types[i % len(os_types)]
        power = power_states[i % len(power_states)]

        cpus = random.choice([2, 4, 8, 16])
        cores_per_socket = random.choice([1, 2])
        sockets = cpus // cores_per_socket
        mem_mb = random.choice([2048, 4096, 8192, 16384, 32768])

        num_disks = random.randint(1, 3)
        disk_sizes = [random.choice([50, 100, 200, 500, 1000]) for _ in range(num_disks)]
        total_disk = sum(disk_sizes)

        num_nics = random.randint(1, 2)
        ip_base = f"10.10.{100 + (i % 4)}"
        ip_addr = f"{ip_base}.{10 + i}"

        ds = datastores_data[i % len(datastores_data)]
        pg = pg_list[i % len(pg_list)]

        create_date = f"2024-{random.randint(1,12):02d}-{random.randint(1,28):02d}T{random.randint(0,23):02d}:00:00"

        # vInfo row
        vinfo.append({
            "VM Name": vm_name,
            "Power State": power,
            "Guest OS": os_type,
            "CPUs": cpus,
            "Cores Per Socket": cores_per_socket,
            "Memory (MB)": mem_mb,
            "Total Disk (GB)": total_disk,
            "NICs": num_nics,
            "IP Addresses": ip_addr if power == "running" else "",
            "Host": host["Host Name"],
            "Cluster": cluster,
            "VM Tools": "running" if power == "running" else "not running",
            "Description": f"{vm_name} server",
            "Create Date": create_date,
            "URN": f"urn:sites:1:vms:{3000 + i}",
        })

        # vCPU row
        vcpu.append({
            "VM Name": vm_name,
            "Total CPUs": cpus,
            "Cores Per Socket": cores_per_socket,
            "Sockets": sockets,
            "CPU Reservation (MHz)": 0,
            "CPU Limit (MHz)": -1,
            "CPU Weight": random.choice([500, 1000, 2000]),
            "Host": host["Host Name"],
            "Cluster": cluster,
        })

        # vMemory row
        vmemory.append({
            "VM Name": vm_name,
            "Memory (MB)": mem_mb,
            "Reservation (MB)": 0,
            "Limit (MB)": -1,
            "Weight": random.choice([500, 1000, 2000]),
            "Host": host["Host Name"],
            "Cluster": cluster,
        })

        # vDisk rows (one per disk)
        for d_idx, d_size in enumerate(disk_sizes):
            vdisk.append({
                "VM Name": vm_name,
                "Disk Name": f"disk-{d_idx}",
                "Capacity (GB)": d_size,
                "Provisioning Type": random.choice(["thin", "thick"]),
                "Datastore": ds["Datastore Name"],
                "Bus Type": "SCSI",
                "Sequence Num": d_idx,
                "URN": f"urn:sites:1:vms:{3000 + i}:volumes:{d_idx}",
            })

        # vNetwork rows (one per NIC)
        for n_idx in range(num_nics):
            mac = f"28:6E:D4:{random.randint(10,99)}:{random.randint(10,99)}:{random.randint(10,99)}"
            vnetwork.append({
                "VM Name": vm_name,
                "NIC Name": f"nic-{n_idx}",
                "MAC Address": mac,
                "Port Group": pg[0],
                "IP Address": ip_addr if n_idx == 0 and power == "running" else "",
                "NIC Type": "VMXNET3",
                "Connected": True if power == "running" else False,
                "URN": f"urn:sites:1:vms:{3000 + i}:nics:{n_idx}",
            })

    # ── Clusters summary ─────────────────────────────────
    vcluster = []
    for cl_name in clusters:
        cl_hosts = [h for h in hosts_data if h["Cluster"] == cl_name]
        vcluster.append({
            "Cluster Name": cl_name,
            "Description": f"Cluster for {cl_name.replace('CL-', '').lower()} workloads",
            "Tag": cl_name.replace("CL-", ""),
            "HA Enabled": True,
            "DRS Enabled": True,
            "Total Hosts": len(cl_hosts),
            "URN": f"urn:sites:1:clusters:{cl_name}",
        })

    return {
        "vInfo": vinfo,
        "vCPU": vcpu,
        "vMemory": vmemory,
        "vDisk": vdisk,
        "vNetwork": vnetwork,
        "vHost": hosts_data,
        "vCluster": vcluster,
        "vDatastore": datastores_data,
        "vSwitch": vswitch_data,
    }
