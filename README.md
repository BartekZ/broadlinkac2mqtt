# BroadlinkAC2MQTT
Control your broadlink-based air conditioner using Home Assistant

![Image](image.png)

## Advantages

* Small application size (~10.2 Mb docker, ~8.2 Mb Windows Standalone)
* Easy to install and use
* Support all platforms
* Parallel independent air conditioning support.
  If one air conditioner is offline, it will not affect the rest!

## Configuration

You must specify the mqtt and air conditioner settings in the config.yml or config.json (less priority) file in the config folder.

Example of config.yml 

```
    service:
      update_interval: 10 # In seconds. Default: 10
      log_level: error    # Supported: info, disabled, fatal, debug, error. Default: error
    
    mqtt:
      broker: "mqtt://192.168.1.10:1883"              # Required. Use mqtts:// for ssl support
      user: admin                                     # Optional  
      password: password                              # Optional    
      client_id: airac                                # Default: broadlinkac
      topic_prefix: aircon                            # Default: airac
      auto_discovery_topic: homeassistant             # Optional
      auto_discovery_topic_retain: false              # Default: true
      certificate_authority: "./config/cert/ca.crt"   # Optional. CA certificate in CRT format.
      skip_cert_cn_check: false                       # Default: true. Don’t verify if the common name in the server certificate matches the value of broker.
      certificate_client: "./config/cert/client.crt"  # Optional. Authorization using client certificates
      key-client: "./config/cert/client.key"          # Optional. Authorization using client certificates
    
    devices:
      - ip: 192.168.1.12
        mac: 34ea345b0fd4   # Only this format is supported
        name: Childroom AC
        port: 80 
      - ip: 192.168.1.18
        mac: 34ea346b0mks   # Only this format is supported
        name: Bedroom AC
        port: 80 
        # Temperature Unit defines the temperature unit of the device, C or F.
        # If this is not set, the temperature unit is Celsius.
        temperature_unit: C
        # Invert display flips ON/OFF mapping for devices with reversed screen logic.
        # Default (false)
        invert_display: false

    # Experimental AUX / AC Freedom cloud support. Local devices keep working
    # when this block is omitted or enabled: false.
    # cloud:
    #   enabled: false
    #   email: "your@email.com"
    #   password: "your_password"
    #   region: eu                 # eu, usa, or cn
    #   auto_discover: true
    #   hidden_devices: []
    #   temperature_unit: C

```

## Installation

### Home Assistant Add-on

Install Add-On:

* Settings > Add-ons > Plus > Repositories > Add `https://github.com/ArtemVladimirov/hassio-add-ons`
* broadlinkac2mqtt > Install > Start

### Docker Compose

```
    version: '3.5'
    services:
      broadlinkac2mqtt:
        image: "ghcr.io/artemvladimirov/broadlinkac2mqtt:latest"
        container_name: "broadlinkac2mqtt"
        restart: "on-failure"
        volumes:
            - /PATH_TO_YOUR_CONFIG:/config     

```

### Docker

```
   docker run -d --name="broadlinkac2mqtt" -v /PATH_TO_YOUR_CONFIG:/config --restart always ghcr.io/artemvladimirov/broadlinkac2mqtt:latest   
```

### Standalone application

Download application from releases or build it with command "go build". Then you can run a program. The config folder must be located in the program folder

## Known issues

### Checksum is incorrect 
 

> [@cHunter789](https://github.com/ArtemVladimirov/broadlinkac2mqtt/issues/6#issuecomment-2308999367) wrote:
>
> if there is a problem with checksum you have to remove the device from ac freedom app, reset wifi and after the wifi is connected once again (in the ac freedom app) just cancel and you will get connection. If it's still 
> not working, just take a look at [a hardware approach](https://github.com/GrKoR/esphome_aux_ac_component/blob/06388ebb2c2792098e93dd844c3c812440a06288/README-EN.md#esphome-aux-air-conditioner-custom-component-aux_ac)

OR your device is not supported.

## AUX / AC Freedom Cloud (experimental)

Some AUX units are reachable only through the AC Freedom cloud. This integration is **opt-in**, uses the undocumented AUX Cloud HTTPS API, and is not a generic Broadlink Cloud client.

Requirements:

* An AC Freedom account
* Internet access from the host that runs broadlinkac2mqtt
* Region `eu`, `usa`, or `cn`

Enable cloud auto-discovery while keeping local devices:

```
    cloud:
      enabled: true
      email: "your@email.com"
      password: "your_password"
      region: eu
      auto_discover: true
      hidden_devices:
        - "Bedroom AC"
      temperature_unit: C
```

Local `devices` stay as they are. Discovered cloud units are added automatically and identified in MQTT/Home Assistant by their MAC. If a local device already uses that MAC, the local device wins.

`device_id` is the AUX/AC Freedom `endpointId` from your account. Leave it empty when `auto_discover: true`: the app fills it in from discovery. Set it only if you add a cloud unit manually:

```
    devices:
      - transport: cloud
        mac: 34ea345b0fd4
        name: Living Room AC
        device_id: "1a2b3c4d-5e6f-7g8h-9i0j-1k2l3m4n5o6p"
```

Local devices do not need `device_id` or `transport`. `ip`, `mac`, and `port` are enough.

Limitations:

* Only AUX/AC Freedom devices that appear in account discovery and support `DNA.KeyValueControl`
* Cloud swing is on/off, not the 8 local vane positions
* The vendor API, app headers, and license query parameter can change or stop working without notice
* Account password is stored in the config file; treat it as a secret

## Support

To motivate the developer, click on the STAR ⭐. I will be very happy!
