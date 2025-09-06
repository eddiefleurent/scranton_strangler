# Short Strangles are completely OP | Theta Gang - YouTube

https://www.youtube.com/watch?v=Z71CUXQZLH4

## Transcript

This video is sponsored by Tendees. Their free app includes trade data that other platforms make you pay for, and only a fool or a sadist would open a strangle without first checking Tendees' free IV rank readings. Click the link in my description to learn more.

Hey team, in this video we are going to get back to our Theta Gang roots and talk about my favorite option strategy since mid-2021: the short strangle. This is a great strategy when you have a generally neutral outlook for the market, but you can adjust and skew your position to take on a bearish or bullish slant as you so desire. This strategy will also work to your advantage in the case of IV crush.

My methodology is heavily derived from the experts at Tastytrade, especially Dr. Jim Schultz. I'll link his videos here, and I strongly recommend checking them out after this. I've been using this strategy for about one year and I can't believe I resisted exploring it for so long before I finally got to it. I'm really excited to share this strategy with you guys, so let's get started.

We create a short strangle by selling an out-of-the-money put and out-of-the-money call to collect premium, and then managing our position over the next few weeks.

Step one is to sell an out-of-the-money put option to collect premium, and step two we flip to calls and sell an out-of-the-money short call to collect more premium. We've now collected a nice stack of cash, but we are short on both ends. As time passes, theta will continue to work in our favor to drive down the prices on both short contracts. If implied volatility drops, then that will also reduce the value of these contracts, which works in our favor. This is known as IV crush, and we'll talk more about that later.

As the days go on, we will seek to make adjustments to our strikes as needed to maximize our chances of walking away with a nice bag. But obviously there are a lot of factors at play here. The strategy involves management and carries risk, but extensive back testing at Tastytrade determined that it is possible to achieve profitable trades over ninety percent of the time by sticking to a few management principles. This doesn't mean that we will collect max gain ninety percent of the time, just that we will close green nine out of ten trades, and that's still a remarkable win percentage. And I wouldn't believe it myself if I didn't spend about a year replicating their results.

Let's not get crazy though—this is a complicated strategy and that ten percent losing rate can be disproportionately costly if we screw up. So let's dive into a real-world example and play this out.

When we use short strangles, we really need to make sure that the underlying stock is relatively stable. If the stock moons or tanks 20% in a day like Netflix, we could be in trouble. So when I use short strangles, I tend to stick to indexed ETFs. SPY isn't moving 18% in a week.

Today is Wednesday, February 23rd, 2022, and SPY is trading for about 432 dollars. I'm going to set up a short strangle by selling an out-of-the-money call and put. Keep in mind that strangles involves selling naked calls and puts, which Robin Hood doesn't allow, so we will paper trade this on Robin Hood and then in my real Merrill Edge account I'll enter a similar position.

I'll let these trades run to expiration before I release this video so we can get a real look at how this all plays out in practice. According to Tastytrade's back testing, in my own experience, selecting a contract 45 days to expiration is an ideal target. That gives you a higher premium on both ends and gives you a lot more management options when the stock starts moving.

So let's flip to April 14th—that's a little more than 45 days out, but it'll do.

Step one is to select a short put and sell it to collect premium. We want to sell out-of-the-money strikes to ensure that we are selling all extrinsic value. As time passes, then potentially all of this premium that I'm about to sell can melt off, putting the gains straight into my pocket.

Strangle traders generally sell either the 30 delta or the 16 delta short strikes. I trade both, and I'm getting into more of the 16 delta now, but let's continue with the 30 delta example.

So let's click around and find the 30 delta put. It looks like that's the 409 strike, so we will sell that and collect about 960 dollars premium.

Breakeven so far is a little under 400. Then I will flip to calls and sell the same 30 delta strike, that will mean selling the 448 strike to collect about another 590 in premium.

Now you'll see with a stock price of about 432, the 30 delta put is farther away and offers more premium. That's totally normal—stocks generally fall a lot faster than they rise, so put premiums are almost always more richly priced than calls. Our breakevens are now 393 and 463.

That puts us pretty darn close to new 52-week highs and lows, so we've got some buffer on either end. With the 30 delta strikes, I've collected a pretty high premium here, but as we'll talk about in a minute, the 30 delta strikes are not that far away from the stock price, so this will involve more management than choosing the 16 delta strikes.

Some traders prefer to sell the 16 delta strikes because they're farther out from the current price and therefore less likely to get blown out. In exchange, you collect less premium with these further out-of-the-money strangles, but as you see in the example here, you get wider breakevens with a 16 delta, especially to the put side.

For now, let's progress with my preference: the 30 delta strangles.

So now what? We've set up this nice strangle, collected well over 1500 premium, and now we immediately flip onto defense. In our favor, we have theta pushing down on both of these contracts, but to our disadvantage, we've got all this potential for loss on either end if SPY goes up too far or down. We stand to lose money.

In theory, we can lose unlimited money if SPY goes up to a thousand dollars in the next 45 days, but that's not realistic, and later in this video we'll talk about the realistic max loss situations.

When I say we must flip to defense, I mean we must monitor SPY's price and manage our trade. We must defend this position.

Our goal is not to wait all 45 days until expiration to claim max profit. Instead, our goal is to manage this position until we are at fifty percent profit. This usually takes about three to four weeks.

I think the best way to describe managing strangles is to use the analogy of football. That's American football, to you wankers in Europe—not soccer. If you're not familiar with American football, you just need to know that you get four attempts called downs to move nine meters toward your goal. If you go nine meters, you get another four attempts—it's called a first down. If you don't know what a touchdown is, I invite you to unsubscribe now.

When we set up our strangle at the 30 or 16 delta strike prices—the choice is yours—we are on first down. Unless you're a 12-year-old playing Madden, you're generally not looking for a touchdown on every play. You're more likely looking for a first down, and for us that means reaching 50% profit, closing the trade, and setting up a new strangle another 45 days out and at new strike prices. That's our goal. Some people like to take profit at 25%—that's up to you.

On first down, all we have to do is let SPY move around inside our strikes and let theta do its work to drive these premiums down so we can walk away with a gain. If you are so lucky that SPY stays inside your strikes until you hit 50% profit, then congratulations—great trade, take your win and start with a new first down.

But what if that doesn't happen? What if SPY makes a move to the downside and starts tapping on this short put? What do we do now?

Now we are on to second down. We're still in a profitable position, but we want to take preventive measures to make sure our breakeven down here does not end up getting punctured.

So on second down, here's what we do: Our short call is now pretty deep out of the money. It probably lost about 70% of its value already, so we are going to buy back this short call for a 70% gain and then we will sell the new 30 delta strike with the same expiration. Since we are rolling down the strike, we will collect more premium and this will expand our breakeven further to the downside. This gives us a nicer buffer against further decline.

With any luck, SPY will come back up or at least stop declining, and that will give us a better chance of ditching with a 50% gain so we can start first down again.

But what if SPY doesn't cooperate and continues declining to the point it's tapping on our original breakeven? Now we're on third down.

At this point, we are once again looking at a 70% gain on our short call, so it's time to roll down and collect additional premium. So we will close this out and take our gain, and then we will roll the short call down to the same strike as our short put. We are rolling into a short straddle, and this will give us yet more premium to expand our breakeven further still.

You very likely made more than 100% of your original short call's value, but we also have this short put with a huge unrealized loss. Our desire at this point is for SPY to recover toward our strikes so that we can clear out some of this red on our short put.

Remember, you've taken a large gain on the calls side already, plus SPY is still inside our new breakeven, so we are still profitable.

If SPY stops declining, if SPY can just come back up to where we're at 25% gain, then we should take first down and reset our strikes and expiration.

But let's say that nightmare happens and SPY goes downhill further still. It is now tapping on our new breakeven, and we are now risking an overall loss. We are now on fourth down.

In football, you've usually got three choices: you can go for a field goal, you can go for it and try to get a first down, or you can punt. We've got the same choices in strangle trading.

**Choice one** is go for a field goal. For non-Americans, this means we're going to go for half points because we don't think we can score fully anymore. At this point, our goal is just to get back to green so we can close for a gain.

To do so, we will roll down our short call further below our short put and collect more premium. This way we have set up an inverted strangle. It is impossible to collect full premium now because one of these will always be in the money no matter what SPY does, but by collecting more credit we can expand our breakeven further to the downside.

And if the stock just cooperates a little bit, we can close green or at least reduce our losses. This is a risky choice though—if SPY keeps dropping you'll keep losing money, and if SPY recovers you risk going in the money on both sides. This can be a good thing because you'll certainly be at an overall profit, but as expiration gets closer you risk early assignment, and if you do let it run to expiration you'll get assigned on both ends which can be costly to untangle.

Once you're in this spot, take the trade off as soon as you're satisfied with the result and go back to first down.

**Choice two**: We're going for it on fourth down. We're going to keep our straddle open and wait for a recovery.

If you're selling 30 delta strangles and the stock drops or rises enough to bust your breakeven, that means the stock has moved much more than it normally does when your trade was open. This is even more true if you're trading the 16 delta strangles.

After such a large move, it's not uncommon for the stock to start either fading its rally or recovering its rapid drop. In our SPY example, we've already rolled down our call a couple times so we are sitting green on realized gains already. If we can hang on to the straddle and the stock recovers near our strike, we can take this trade off for potentially even more of a gain than we had with our original strangle.

There is also risk to this of course—if SPY keeps dropping we will continue losing money on the put side much faster than we make money on the call side.

**Choice three**: We can punt. If we are willing to roll our expiration out further in time, we can collect more premium and reposition our strikes to a more favorable situation. We can trade time for premium, although we may not get back to a 30/30 strangle. We shouldn't have too much trouble rolling to more favorable strikes.

If we add a month to this trade, contracts with more time to expiration are always more valuable than those that are about to expire, so we can almost certainly expect to roll for a credit.

The risk with this is that we are adding potentially a lot of time to the trade. Depending on what the stock does from here, you might have just been better off closing the original trade for a loss and then trying again. Don't immediately assume it's best to punt because you may be able to recover your losses quicker by just closing for a loss and going back to first down.

Now we can't talk strangles without talking implied volatility—that's IV.

On stocks that are volatile like Tesla, options trade at very high premiums since option buyers know that a stock can move 10% in a day like it's nothing. On low volatility stocks like Coca-Cola, premiums are dirt cheap because option buyers aren't paying top dollar for out-of-the-money contracts when the stock hardly ever moves.

But just as important, we must remember that a stock's own IV will move throughout the year ahead of earnings. AMD's IV spikes because its earnings release will move the stock tremendously. Because implied volatility spikes, options get expensive around this time, and AMD premiums start looking like Tesla's.

If you sell options when IV is high, you can benefit from IV dropping later. This is what we call IV crush. AMD premiums start looking more like Coca-Cola's after IV crush.

The ideal situation would be to identify when a stock's IV is high so we can sell strangles and then buy it back when IV drops.

To identify when IV is high, we should use IV rank—that's IVR. That will tell us how a stock's current IV compares to its overall historical IV. An IVR of 90 will tell us that the stock is in its top 10% most volatile states, so premiums will be high and we can benefit from IV crush when it reverts back to its norm.

We should be aware that this probably means that the stock has big news like earnings coming, which will affect price action, but no matter why IV is high, we stand to benefit when it falls.

But IVR isn't a commonly reported statistic. Where do we find a stock's IVR? Sometimes brokerages will offer it like Tastyworks or Thinkorswim, but I'm not going to open a brokerage account just to find IVR data. There are some other websites that offer it, but I'm not willing to pay for this.

To get IVR, I introduced Tendees. Tendees expects to roll out its IV rank feature by the end of May, and this is going to make it much easier for us to plan our strangles.

The image my man on the inside sent is low res because he took it with a spy cam, but we're gonna make it work. Beside every ticker you search, you'll get the stock's current IV rank, its current implied volatility, and a nice ladder to show you where this falls versus the 52-week high and low.

Having the high end low will inform us on how tight or loose this IV range actually is, so we can get a better read on how much IV can actually fall. In addition, we'll see the highest IV options for each ticker so we can maximize our IV crushing.

Personally, I'm really looking forward to this volatility screen to get IV rank. Right now I have to dig through a bunch of websites that look like they were designed in 2003. This screen right here is worth gold, and I'm absolutely going to plan my strangles based on this data, and I'm serious about that too.

Tendees is right here on my home screen, and when you're in need of real-time order flow, Tendees has you covered with Tendees Flow. Bounce ideas off one another in Tendees rooms for each ticker, either in voice or text. And the best part: Tendees prides itself on being 100% free forever with no ads.

Tendees download now through my link in the description—only on iOS, Android coming soon. I promise.

That strangles in a nutshell. Now let's address some common questions and best practices.

**Number one**: Is loss really unlimited? Yes, in theory you can lose all your money and go into debt if the stock absolutely moons or if it goes bankrupt. But realistically, this is not going to happen unless you're strangling the most volatile possible stocks. Tesla around earnings might be an example of this risk, or maybe one of the bitcoin miners like Riot.

This is why I stick to the indexed ETFs for strangling. We know SPY isn't going to 500 or 300 in the next 30 days. Yes, I know it's theoretically possible—just like driving a car is theoretically unlimited risk because you can die. This doesn't stop us from driving, and some of you guys probably do it drunk.

The 2020 crash in March lost about 12% on the S&P, and the April recovery was also about 12%, so if you held you actually might be all right on your strangles. But nobody held through that mess.

So your realistic max loss on a short put at the 30 delta strike would have been about 70%. If you rolled your calls effectively, you could knock that down to maybe 55% to 60%. That's really shitty no matter how you approach it, so those black swan events can do some real damage.

For that reason, just take the trade off and wait for the market to get back to normal. If you're sitting on a 200% loss, a 200% loss is manageable, but a 700% loss would be devastating.

Use stop losses. Also, if the US-indexed ETFs are too expensive, consider the other indices like Brazil's EWZ, which is what I started with, or VEU, which is the whole world minus the US.

**Number two**: Should we hold and aim for 100% gain at expiration? Usually not. I understand the temptation though—it might take 30 days on a 45-day contract to hit 50% gain, so why not wait another two weeks and get the other 50% gain?

The reason we don't do that is because the near-expiration contracts have a lot of gamma risk. That means if the stock makes a sharp move in either direction, you'll lose more money than if it did so when expiration was further away. Also, you have less time to roll your strikes when you're close to expiration.

In general, it's best to close if you're at 50% gain or if you're two weeks from expiration.

**Number three**: Should you use the 16 or 30 delta strangles? I generally prefer the 30 delta strangles in the beginning, but now I'm starting to see that there's more attraction to the 16s. I like the wider breakevens and having to do less management than when I use the 30s. However, you get a ton more premium in the 30s.

My recommendation is to experiment with both of these and see which one you like best. Rest assured, you can't go wrong.

Now, after all this, let's come back and see how our theoretical 30 delta strangle on SPY turned out. Right now it is April 14th, expiration day. Despite some big ups and downs caused by Russia invading Ukraine, the 30 delta strangle at 409 by 448 would have yielded 100%.

It looked like we might pierce 409 at one point, but the market recovered and pierced through our 448 short call at the end of March. At that point we could have closed for about 40% gain, or we could have moved to second down and rolled our 409 put to about 420 for more premium.

From there we could have held until the April 14th expiration if we wanted to and made even more than our original 1,548 open. Despite some wild price action, this 30 delta held nicely.

On my Merrill Edge account: On March 3rd, I opened a 16 delta 400 by 465 strangle and collected 715 dollars premium. Expiration day was April 14th. On March 17th, I closed for 272 dollars for a gain of 443%, or 61%. I stayed on first down the whole time.

On March 18th, I opened a 400 by 465 strangle for 504 dollars premium. Expiration was April 29th. If I had just held it, I would have banked the full premium, and if I took gains at 50%, I would have closed at the end of March without ever leaving first down.

But I made the bad assumption that the market would keep rising, so I rolled my 400 put to 440, which backfired. Despite some more adjustment, I still ended up with an 898 loss.

On the one hand, this is the result of me trading poorly and trying to push for more gains when I should have done less. Rolling from 400 to 440 on the put side was very aggressive, but I do think this shows something important about how much lower the risk is on strangles than people think.

Despite the S&P having the worst month since the 2020 limit-down crash, my total loss is only 900%, and only then because I rolled too aggressively. If I held my original strangle, I would have made max profit.

I'm kicking myself for trading like a donkey, but that's part of the game. Sometimes you trade like a donkey. Let my sacrifice show that it's best not to get greedy or think you know better than the Greeks. Just stick to the plan and you'll avoid this messy trade I got myself into.

For what it's worth, my last non-professional recommendation is this: Open a 16 delta strangle on EWZ to get started. It's a cheap stock, and Brazil isn't going to implode in the next 30 days nor is it going to join the G7.

I started with strangles on EWZ and made enough profit to buy 100 shares by just picking up a few at a time with the profit. I also did the same with FXI, that's China, which I've since rolled into VEU, an international ETF. I can't really recommend China right now since it's really having some serious problems, but if you use EWZ you should be in pretty good shape.

I bet if you start with EWZ as well, you'll be successful.

That's all for this video. Find me and the rest of Theta Gang on Discord, and don't forget to join Tendees for the free IVR data. Good luck trading, and I'll see you next time.
