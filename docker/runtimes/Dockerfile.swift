# Swift runtime image
FROM swift:6.1

# Install nova-agent
COPY bin/nova-agent /usr/local/bin/nova-agent

# Create directories
RUN mkdir -p /code /tmp && chmod 1777 /tmp

# Set environment for Docker mode
ENV NOVA_AGENT_MODE=tcp
ENV NOVA_SKIP_MOUNT=true
ENV SWIFT_ROOT=/usr
ENV PATH="/usr/local/bin:/usr/bin:/bin"

EXPOSE 9999

CMD ["/usr/local/bin/nova-agent"]
