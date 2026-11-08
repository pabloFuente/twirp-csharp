using System.Net;
using Acme.Inventory.Proto;
using Acme.Clients;
using Acme.Shop.V1;
using Google.Protobuf.WellKnownTypes;

namespace Acme.Tests;

/// <summary>Calls through the generated AcmeApi class (namespace=Acme.Clients, className=AcmeApi).</summary>
public class ClientTests
{
    [Fact]
    public async Task Calls_PostProtobufToTheRoute()
    {
        var handler = new StubHandler
        {
            Respond = _ => Stubs.Protobuf(new Item { Id = "42", Name = "Lamp" }),
        };
        using var client = Stubs.Client(handler);

        var item = await AcmeApi.GetItem(client, new GetItemRequest { Id = "42" });

        Assert.Equal("Lamp", item.Name);
        var request = handler.LastRequest!;
        Assert.Equal(HttpMethod.Post, request.Method);
        Assert.Equal("/twirp/acme.shop.v1.Shop/GetItem", request.RequestUri!.AbsolutePath);
        Assert.Equal("application/protobuf", request.Content!.Headers.ContentType!.MediaType);
        Assert.Equal("42", GetItemRequest.Parser.ParseFrom(handler.LastBody).Id);
    }

    [Fact]
    public void Routes_AreConstants()
    {
        Assert.Equal("/twirp/acme.shop.v1.Shop/GetItem", AcmeApi.Routes.Shop.GetItem);
        Assert.Equal("/twirp/acme.inventory.Inventory/Count", AcmeApi.Routes.Inventory.Count);
        Assert.Equal("rpc/acme.shop.v1.Shop/GetItem", LegacyApi.Routes.Shop.GetItem);
    }

    [Fact]
    public async Task RelativePathPrefix_KeepsTheSubpathOfTheBaseAddress()
    {
        var handler = new StubHandler { Respond = _ => Stubs.Protobuf(new Item()) };
        using var client = Stubs.Client(handler, "http://server/base/");

        await LegacyApi.GetItem(client, new GetItemRequest());

        Assert.Equal(
            "/base/rpc/acme.shop.v1.Shop/GetItem",
            handler.LastRequest!.RequestUri!.AbsolutePath
        );
    }

    [Fact]
    public async Task ConfigureRequest_SetsHeadersOnThatRequestOnly()
    {
        var handler = new StubHandler { Respond = _ => Stubs.Protobuf(new Item()) };
        using var client = Stubs.Client(handler);

        await AcmeApi.GetItem(
            client,
            new GetItemRequest(),
            request => request.Headers.Authorization = new("Bearer", "token-1")
        );
        Assert.Equal("Bearer token-1", handler.LastRequest!.Headers.Authorization!.ToString());

        await AcmeApi.GetItem(client, new GetItemRequest());
        Assert.Null(handler.LastRequest!.Headers.Authorization);
        Assert.Empty(client.DefaultRequestHeaders);
    }

    [Fact]
    public async Task CancellationToken_CancelsTheCall()
    {
        var handler = new StubHandler { Delay = TimeSpan.FromSeconds(30) };
        using var client = Stubs.Client(handler);
        using var cancellation = new CancellationTokenSource(TimeSpan.FromMilliseconds(50));

        await Assert.ThrowsAnyAsync<OperationCanceledException>(() =>
            AcmeApi.GetItem(client, new GetItemRequest(), cancellationToken: cancellation.Token)
        );
    }

    [Fact]
    public async Task ServiceOfAPackageNamedSystem_IsUsable()
    {
        // The client lives in Acme.Clients, next to Acme.System: a type written System.X in it
        // would resolve to Acme.System.X
        var handler = new StubHandler { Respond = _ => Stubs.Protobuf(new Acme.System.Health { Ok = true }) };
        using var client = Stubs.Client(handler);

        Assert.True((await AcmeApi.Check(client, new Acme.System.Health())).Ok);
        Assert.Equal("/twirp/acme.system.Monitor/Check", handler.LastRequest!.RequestUri!.AbsolutePath);
    }

    [Fact]
    public async Task TypesOfOtherPackagesAndNestedTypes_AreUsable()
    {
        var handler = new StubHandler();
        using var client = Stubs.Client(handler);

        handler.Respond = _ => Stubs.Protobuf(new Empty());
        Assert.NotNull(await AcmeApi.Ping(client, new Empty()));
        Assert.Equal(
            "/twirp/acme.shop.v1.Shop/Ping",
            handler.LastRequest!.RequestUri!.AbsolutePath
        );

        handler.Respond = _ => Stubs.Protobuf(new Order.Types.Line.Types.Price { Cents = 1250 });
        var price = await AcmeApi.Quote(
            client,
            new Order.Types.Line { ItemId = "42", Quantity = 2 }
        );
        Assert.Equal(1250, price.Cents);

        // A service of another file, whose own csharp_namespace names its types
        handler.Respond = _ => Stubs.Protobuf(new Stock { Count = 7 });
        Assert.Equal(7, (await AcmeApi.Count(client, new Item { Id = "42" })).Count);
        Assert.Equal(
            "/twirp/acme.inventory.Inventory/Count",
            handler.LastRequest!.RequestUri!.AbsolutePath
        );

        // An editions file
        handler.Respond = _ => Stubs.Protobuf(new Acme.Editions.Message { Text = "hi" });
        Assert.Equal(
            "hi",
            (await AcmeApi.Say(client, new Acme.Editions.Message { Text = "hi" })).Text
        );
    }

    /// <summary>
    /// A UI-like context whose only thread is blocked by the caller: whatever is posted to it never runs.
    /// </summary>
    private sealed class BlockedContext : SynchronizationContext
    {
        public override void Post(SendOrPostCallback d, object? state) { }
    }

    [Fact]
    public void BlockingOnACall_FromASynchronizationContext_DoesNotDeadlock()
    {
        var handler = new StubHandler { Delay = TimeSpan.FromMilliseconds(50) };
        using var client = Stubs.Client(handler);
        var previous = SynchronizationContext.Current;
        SynchronizationContext.SetSynchronizationContext(new BlockedContext());
        try
        {
            var call = AcmeApi.Ping(client, new Empty());
            // Blocking on the call is what this test is about
#pragma warning disable xUnit1031
            Assert.True(
                call.Wait(TimeSpan.FromSeconds(10)),
                "the call waits for the blocked context"
            );
#pragma warning restore xUnit1031
        }
        finally
        {
            SynchronizationContext.SetSynchronizationContext(previous);
        }
    }
}
