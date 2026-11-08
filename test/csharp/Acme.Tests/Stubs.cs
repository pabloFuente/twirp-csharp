using System.Net;
using Google.Protobuf;

namespace Acme.Tests;

/// <summary>
/// Records each request and answers it with <see cref="Respond"/>. It awaits with
/// ConfigureAwait(false): HttpClient runs the handler on the caller's thread.
/// </summary>
public sealed class StubHandler : HttpMessageHandler
{
    public Func<HttpRequestMessage, HttpResponseMessage> Respond { get; set; } =
        _ => Stubs.Protobuf(new Google.Protobuf.WellKnownTypes.Empty());

    public TimeSpan Delay { get; set; }

    public HttpRequestMessage? LastRequest { get; private set; }

    // Read while the request is in flight: the client disposes the content afterwards
    public byte[]? LastBody { get; private set; }

    protected override async Task<HttpResponseMessage> SendAsync(
        HttpRequestMessage request,
        CancellationToken cancellationToken
    )
    {
        LastRequest = request;
        LastBody = await request
            .Content!.ReadAsByteArrayAsync(cancellationToken)
            .ConfigureAwait(false);
        if (Delay > TimeSpan.Zero)
        {
            await Task.Delay(Delay, cancellationToken).ConfigureAwait(false);
        }
        return Respond(request);
    }
}

public static class Stubs
{
    public static HttpClient Client(StubHandler handler, string baseAddress = "http://server/") =>
        new(handler) { BaseAddress = new Uri(baseAddress) };

    public static HttpResponseMessage Protobuf(IMessage message) =>
        new(HttpStatusCode.OK) { Content = new ByteArrayContent(message.ToByteArray()) };

    public static HttpResponseMessage Error(int status, string body) =>
        new((HttpStatusCode)status) { Content = new StringContent(body) };
}
