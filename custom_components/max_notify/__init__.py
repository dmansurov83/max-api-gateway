"""MAX Notify integration for Home Assistant.

Sends notifications through the MAX Gateway HTTP API.

Configuration (configuration.yaml):

    notify:
      - platform: max_notify
        name: max
        url: "http://192.168.1.100:8000"
        api_key: "ваш_api_token"
        chat_id: -1234567890

To send an image, pass it in the `data` of the notify service:

    action:
      - service: notify.max
        data:
          message: "Снимок с камеры"
          image: "/config/www/snapshot.jpg"

`image` may also be an http(s) URL — it will be downloaded and sent:

    action:
      - service: notify.max
        data:
          message: "Снимок с камеры"
          image: "http://192.168.1.50:8123/local/snapshot.jpg"
"""

from __future__ import annotations

import logging

from homeassistant.const import Platform
from homeassistant.core import HomeAssistant
from homeassistant.helpers.typing import ConfigType

_LOGGER = logging.getLogger(__name__)

DOMAIN = "max_notify"
PLATFORMS = [Platform.NOTIFY]


async def async_setup(hass: HomeAssistant, config: ConfigType) -> bool:
    """Set up the MAX Notify component."""
    return True


async def async_setup_entry(hass: HomeAssistant, entry) -> bool:
    """Set up MAX Notify from a config entry."""
    await hass.config_entries.async_forward_entry_setups(entry, PLATFORMS)
    return True


async def async_unload_entry(hass: HomeAssistant, entry) -> bool:
    """Unload a config entry."""
    return await hass.config_entries.async_unload_platforms(entry, PLATFORMS)


async def async_setup_platform(hass, config, async_add_entities, discovery_info=None):
    """Set up the MAX Notify platform via configuration.yaml."""
    from .notify import MAXNotify

    async_add_entities([MAXNotify(config)], update_before_add=False)